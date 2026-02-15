package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/browser"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/gateway"
	"github.com/user/talon/internal/llm"
	"github.com/user/talon/internal/session"
	"github.com/user/talon/internal/speech"
	"github.com/user/talon/internal/tools"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main Wails application struct. Its exported methods are
// bound to the frontend and callable from JavaScript.
type App struct {
	ctx       context.Context
	agentCfg  config.AgentConfig
	deps      agent.Deps
	sessionID string
	ready     bool
	initError string

	// Browser manager lifecycle.
	browserMgr *browser.Manager

	// Telegram bot lifecycle.
	tgMu     sync.Mutex
	tgCancel context.CancelFunc // cancels the running Telegram goroutine
	tgStatus string             // "running", "stopped", "error: ..."

	// Stream cancellation.
	streamMu     sync.Mutex
	streamCancel context.CancelFunc

	// Voice recording state.
	voiceMu            sync.Mutex
	voiceRecordingPath string

	// Dictation timing (for saving hotkey recordings to flow history).
	dictStartTime time.Time
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called by Wails when the application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Try loading .env files for convenience (Mac apps launched from
	// Finder don't inherit shell environment variables).
	loadDotEnv(".env")
	if home, err := os.UserHomeDir(); err == nil {
		loadDotEnv(filepath.Join(home, ".talon", ".env"))
	}

	// Bootstrap ~/.talon/ directory and defaults.
	baseDir, err := config.Bootstrap()
	if err != nil {
		a.initError = fmt.Sprintf("Bootstrap failed: %v", err)
		log.Printf("startup error: %s", a.initError)
		return
	}

	// Load configuration.
	cfg, err := config.Load(baseDir)
	if err != nil {
		a.initError = fmt.Sprintf("Config load failed: %v", err)
		log.Printf("startup error: %s", a.initError)
		return
	}

	if cfg.AnthropicKey == "" {
		cfg.AnthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if cfg.AnthropicKey == "" {
		a.initError = "No API key found. Set 'anthropic_key' in ~/.talon/config.json or ANTHROPIC_API_KEY env var."
		log.Printf("startup error: %s", a.initError)
		return
	}

	// Resolve agent config.
	agentCfg, ok := cfg.Agents["main"]
	if !ok {
		a.initError = "Agent 'main' not found in config"
		log.Printf("startup error: %s", a.initError)
		return
	}

	// Initialise core components.
	sessionMgr := session.NewManager(baseDir)
	llmClient := llm.NewClient(cfg.AnthropicKey, agentCfg.Model)
	toolRegistry := tools.NewRegistry()
	tools.RegisterStandardTools(toolRegistry, baseDir)

	// Start browser and register browser tools.
	browserMgr := browser.NewManager()
	if err := browserMgr.Start(cfg.BrowserHeadless); err != nil {
		log.Printf("Warning: browser failed to start: %v (browser tools disabled)", err)
	} else {
		tools.RegisterBrowserTools(toolRegistry, browserMgr)
		a.browserMgr = browserMgr
		log.Printf("Browser started (headless=%t)", cfg.BrowserHeadless)
	}

	a.agentCfg = agentCfg
	a.deps = agent.Deps{
		SessionMgr:   sessionMgr,
		LLMClient:    llmClient,
		ToolRegistry: toolRegistry,
		BaseDir:      baseDir,
	}
	a.sessionID = fmt.Sprintf("%s:gui:%d", agentCfg.SessionPrefix, time.Now().UnixMilli())
	a.ready = true

	log.Printf("Talon GUI ready (agent=%s, model=%s)", agentCfg.Name, agentCfg.Model)

	// Always show the waveform icon in the macOS menu bar while the app is open.
	speech.ShowMenuBarIcon()

	// Auto-start push-to-talk dictation if enabled in config.
	if cfg.HotkeyEnabled {
		a.startDictation(cfg)
	}

	// Auto-start Telegram bot if token is configured.
	if cfg.TelegramToken == "" {
		cfg.TelegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	if cfg.TelegramToken != "" {
		a.startTelegram(cfg)
	} else {
		a.tgStatus = "stopped"
	}
}

// shutdown is called by Wails when the application is closing.
func (a *App) shutdown(ctx context.Context) {
	log.Println("Talon GUI shutting down")
	speech.TeardownDictation()
	speech.HideMenuBarIcon()
	a.stopTelegram()
	if a.browserMgr != nil {
		a.browserMgr.Close()
	}
}

// startTelegram launches the Telegram bot in a background goroutine.
func (a *App) startTelegram(cfg *config.Config) {
	a.tgMu.Lock()
	defer a.tgMu.Unlock()

	// Stop any existing instance first and give it a moment to release
	// the long-poll connection on Telegram's side.
	if a.tgCancel != nil {
		a.tgCancel()
		a.tgCancel = nil
		time.Sleep(500 * time.Millisecond)
	}

	tgCtx, cancel := context.WithCancel(a.ctx)
	a.tgCancel = cancel
	a.tgStatus = "running"

	go func() {
		log.Println("[telegram] starting bot from GUI...")
		gateway.RunTelegram(tgCtx, cfg, a.deps)
		// If RunTelegram returns, the bot has stopped.
		a.tgMu.Lock()
		if a.tgStatus == "running" {
			a.tgStatus = "stopped"
		}
		a.tgMu.Unlock()
		log.Println("[telegram] bot stopped")
	}()
}

// stopTelegram cancels the running Telegram bot goroutine.
func (a *App) stopTelegram() {
	a.tgMu.Lock()
	defer a.tgMu.Unlock()

	if a.tgCancel != nil {
		a.tgCancel()
		a.tgCancel = nil
	}
	a.tgStatus = "stopped"
}

// FileAttachment represents a file uploaded by the user in the chat.
type FileAttachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded file content
}

// ChatStep represents a single intermediate step (thinking, tool call, tool result)
// that occurred during an agent turn. Exposed to the frontend.
type ChatStep struct {
	Type      string `json:"type"`                 // "thinking", "tool_call", "tool_result"
	Content   string `json:"content"`              // thinking text or tool result
	ToolName  string `json:"tool_name,omitempty"`   // for tool_call / tool_result
	ToolInput string `json:"tool_input,omitempty"`  // for tool_call (JSON string)
}

// ChatResponse is the structured response returned to the frontend, containing
// the final text answer along with any intermediate thinking and tool steps.
type ChatResponse struct {
	Steps     []ChatStep `json:"steps"`
	FinalText string     `json:"final_text"`
}

// SendMessage sends a plain text user message to the agent and returns the response.
func (a *App) SendMessage(input string) (*ChatResponse, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}
	content, _ := json.Marshal(input)
	result, err := agent.RunAgentTurn(a.ctx, a.sessionID, content, a.agentCfg, a.deps)
	if err != nil {
		return nil, err
	}
	return turnResultToChat(result), nil
}

// SendMessageWithFiles sends a user message with optional file attachments to the agent.
// Files are sent as multimodal content blocks to the Anthropic API.
func (a *App) SendMessageWithFiles(input string, files []FileAttachment) (*ChatResponse, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}
	content := buildUserContent(input, files)
	result, err := agent.RunAgentTurn(a.ctx, a.sessionID, content, a.agentCfg, a.deps)
	if err != nil {
		return nil, err
	}
	return turnResultToChat(result), nil
}

// SendMessageStream starts a streaming agent turn for a plain text message.
// It returns immediately; results are delivered via "stream:event" Wails events.
func (a *App) SendMessageStream(input string) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	content, _ := json.Marshal(input)
	go a.runStream(content)
	return nil
}

// SendMessageStreamWithFiles starts a streaming agent turn with file attachments.
// It returns immediately; results are delivered via "stream:event" Wails events.
func (a *App) SendMessageStreamWithFiles(input string, files []FileAttachment) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	content := buildUserContent(input, files)
	go a.runStream(content)
	return nil
}

// runStream executes a streaming agent turn, emitting events to the frontend
// as content arrives from the LLM.
func (a *App) runStream(content json.RawMessage) {
	// Create a cancellable child context for this stream.
	streamCtx, cancel := context.WithCancel(a.ctx)
	a.streamMu.Lock()
	a.streamCancel = cancel
	a.streamMu.Unlock()

	defer func() {
		a.streamMu.Lock()
		a.streamCancel = nil
		a.streamMu.Unlock()
		cancel()
	}()

	emit := func(evt agent.StreamEvent) {
		wailsRuntime.EventsEmit(a.ctx, "stream:event", map[string]interface{}{
			"type":       evt.Type,
			"content":    evt.Content,
			"tool_name":  evt.ToolName,
			"tool_input": evt.ToolInput,
		})
	}

	result, err := agent.RunAgentTurnStream(streamCtx, a.sessionID, content, a.agentCfg, a.deps, emit)
	if err != nil {
		wailsRuntime.EventsEmit(a.ctx, "stream:event", map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	// Emit the final done event with complete response.
	chatResp := turnResultToChat(result)
	stepsData := make([]map[string]interface{}, 0, len(chatResp.Steps))
	for _, s := range chatResp.Steps {
		stepsData = append(stepsData, map[string]interface{}{
			"type":       s.Type,
			"content":    s.Content,
			"tool_name":  s.ToolName,
			"tool_input": s.ToolInput,
		})
	}
	wailsRuntime.EventsEmit(a.ctx, "stream:event", map[string]interface{}{
		"type":       "done",
		"final_text": chatResp.FinalText,
		"steps":      stepsData,
	})
}

// turnResultToChat converts an agent TurnResult to a frontend ChatResponse.
func turnResultToChat(r *agent.TurnResult) *ChatResponse {
	cr := &ChatResponse{FinalText: r.FinalText}
	for _, s := range r.Steps {
		cr.Steps = append(cr.Steps, ChatStep{
			Type:      s.Type,
			Content:   s.Content,
			ToolName:  s.ToolName,
			ToolInput: s.ToolInput,
		})
	}
	return cr
}

// buildUserContent constructs the JSON content for a user message.
// For plain text it returns a JSON string; with files it returns an array
// of Anthropic content blocks (image, document, or text).
func buildUserContent(text string, files []FileAttachment) json.RawMessage {
	if len(files) == 0 {
		data, _ := json.Marshal(text)
		return data
	}

	var blocks []map[string]interface{}
	for _, f := range files {
		if isImageMime(f.MimeType) {
			blocks = append(blocks, map[string]interface{}{
				"type": "image",
				"source": map[string]interface{}{
					"type":       "base64",
					"media_type": f.MimeType,
					"data":       f.Data,
				},
			})
		} else if f.MimeType == "application/pdf" {
			blocks = append(blocks, map[string]interface{}{
				"type": "document",
				"source": map[string]interface{}{
					"type":       "base64",
					"media_type": f.MimeType,
					"data":       f.Data,
				},
			})
		} else {
			// Text-based file — decode from base64 and include as text content.
			decoded, err := base64.StdEncoding.DecodeString(f.Data)
			if err != nil {
				decoded = []byte("[failed to decode file]")
			}
			blocks = append(blocks, map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("File: %s\n```\n%s\n```", f.Name, string(decoded)),
			})
		}
	}

	if text != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}

	data, _ := json.Marshal(blocks)
	return data
}

// isImageMime returns true for image MIME types supported by the Anthropic API.
func isImageMime(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// CancelStream cancels the currently running stream, if any.
func (a *App) CancelStream() {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	if a.streamCancel != nil {
		log.Println("Cancelling active stream")
		a.streamCancel()
		a.streamCancel = nil
	}
}

// NewSession starts a fresh conversation session and returns the new session ID.
func (a *App) NewSession() string {
	a.sessionID = fmt.Sprintf("%s:gui:%d", a.agentCfg.SessionPrefix, time.Now().UnixMilli())
	return a.sessionID
}

// GetSessionID returns the current active session ID.
// Returns an empty string if the app hasn't finished starting up yet.
func (a *App) GetSessionID() string {
	if !a.ready {
		return ""
	}
	return a.sessionID
}

// ListSessions returns metadata for all GUI sessions, sorted newest first.
func (a *App) ListSessions() ([]session.SessionInfo, error) {
	if a.deps.SessionMgr == nil {
		return []session.SessionInfo{}, nil
	}
	return a.deps.SessionMgr.ListGUISessions()
}

// HistoryMessage is a simplified message format for loading past sessions
// into the frontend.
type HistoryMessage struct {
	Role    string     `json:"role"`
	Content string     `json:"content"`
	Steps   []ChatStep `json:"steps"`
}

// LoadSession loads a previous session's messages and makes it the active
// session. Returns simplified messages suitable for frontend display.
func (a *App) LoadSession(sessionID string) ([]HistoryMessage, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}

	msgs, err := a.deps.SessionMgr.Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	// Set as active session so subsequent messages continue this conversation.
	a.sessionID = sessionID

	// Convert to frontend-friendly format.
	var result []HistoryMessage
	for _, msg := range msgs {
		parsed := parseMessageForFrontend(msg)
		if parsed != nil {
			result = append(result, *parsed)
		}
	}

	return result, nil
}

// DeleteSession removes a saved session.
func (a *App) DeleteSession(sessionID string) error {
	if a.deps.SessionMgr == nil {
		return fmt.Errorf("app not ready")
	}
	return a.deps.SessionMgr.DeleteSession(sessionID)
}

// parseMessageForFrontend converts a stored session message to a simplified
// HistoryMessage for the frontend. Returns nil for messages that should be
// skipped (e.g. tool_result messages).
func parseMessageForFrontend(msg session.Message) *HistoryMessage {
	if msg.Role == "user" {
		// Check if this is a tool_result message (skip it).
		var blocks []map[string]interface{}
		if json.Unmarshal(msg.Content, &blocks) == nil && len(blocks) > 0 {
			if typ, _ := blocks[0]["type"].(string); typ == "tool_result" {
				return nil
			}
		}
		text := session.ExtractTextFromContent(msg.Content)
		return &HistoryMessage{
			Role:    "user",
			Content: text,
			Steps:   []ChatStep{},
		}
	}

	if msg.Role == "assistant" {
		text := session.ExtractTextFromContent(msg.Content)
		var steps []ChatStep

		// Extract thinking and tool_use steps from content blocks.
		var blocks []map[string]interface{}
		if json.Unmarshal(msg.Content, &blocks) == nil {
			for _, block := range blocks {
				typ, _ := block["type"].(string)
				switch typ {
				case "thinking":
					thinking, _ := block["thinking"].(string)
					steps = append(steps, ChatStep{
						Type:    "thinking",
						Content: thinking,
					})
				case "tool_use":
					name, _ := block["name"].(string)
					inputRaw, _ := json.Marshal(block["input"])
					steps = append(steps, ChatStep{
						Type:      "tool_call",
						ToolName:  name,
						ToolInput: string(inputRaw),
					})
				}
			}
		}

		if steps == nil {
			steps = []ChatStep{}
		}

		return &HistoryMessage{
			Role:    "assistant",
			Content: text,
			Steps:   steps,
		}
	}

	return nil
}

// IsReady returns whether the backend has been successfully initialised.
func (a *App) IsReady() bool {
	return a.ready
}

// GetStatus returns the current app status for the frontend to display.
func (a *App) GetStatus() map[string]interface{} {
	a.tgMu.Lock()
	tgStatus := a.tgStatus
	a.tgMu.Unlock()

	result := map[string]interface{}{
		"ready":          a.ready,
		"agentName":      a.agentCfg.Name,
		"telegramStatus": tgStatus,
	}
	if !a.ready {
		result["error"] = a.initError
	}
	return result
}

// ToggleTelegram starts or stops the Telegram bot from the frontend.
func (a *App) ToggleTelegram() (string, error) {
	a.tgMu.Lock()
	currentStatus := a.tgStatus
	a.tgMu.Unlock()

	if currentStatus == "running" {
		a.stopTelegram()
		return "stopped", nil
	}

	// Load config to get the token.
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	if cfg.TelegramToken == "" {
		return "", fmt.Errorf("no Telegram token configured — add it in Settings")
	}

	a.startTelegram(cfg)
	return "running", nil
}

// SettingsPayload is the structure exposed to the frontend for reading/writing settings.
type SettingsPayload struct {
	AnthropicKey     string `json:"anthropic_key"`
	TelegramToken    string `json:"telegram_token"`
	SpeechProvider   string `json:"speech_provider"` // "whisper" or "deepgram"
	SpeechModel      string `json:"speech_model"`    // "gpt-4o-mini-transcribe", "gpt-4o-transcribe", "whisper-1"
	OpenAIKey        string `json:"openai_key"`
	DeepgramKey      string `json:"deepgram_key"`
	HotkeyEnabled  bool   `json:"hotkey_enabled"`
	HotkeyModifier string `json:"hotkey_modifier"`
}

// GetSettings returns the current API key and Telegram token for the settings UI.
func (a *App) GetSettings() (*SettingsPayload, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	modifier := cfg.HotkeyModifier
	if modifier == "" {
		modifier = speech.DefaultModifier
	}

	return &SettingsPayload{
		AnthropicKey:   cfg.AnthropicKey,
		TelegramToken:  cfg.TelegramToken,
		SpeechProvider: cfg.SpeechProvider,
		SpeechModel:    cfg.SpeechModel,
		OpenAIKey:      cfg.OpenAIKey,
		DeepgramKey:    cfg.DeepgramKey,
		HotkeyEnabled:  cfg.HotkeyEnabled,
		HotkeyModifier: modifier,
	}, nil
}

// SaveSettings persists the API key and Telegram token, then re-initialises
// the LLM client so the change takes effect immediately.
func (a *App) SaveSettings(payload SettingsPayload) error {
	baseDir, err := config.TalonDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.AnthropicKey = payload.AnthropicKey
	cfg.TelegramToken = payload.TelegramToken
	cfg.SpeechProvider = payload.SpeechProvider
	cfg.SpeechModel = payload.SpeechModel
	cfg.OpenAIKey = payload.OpenAIKey
	cfg.DeepgramKey = payload.DeepgramKey
	cfg.HotkeyEnabled = payload.HotkeyEnabled
	cfg.HotkeyModifier = payload.HotkeyModifier

	if err := config.Save(baseDir, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	// Hot-reload: if we have a valid key now, rebuild the LLM client.
	if cfg.AnthropicKey != "" && a.agentCfg.Model != "" {
		a.deps.LLMClient = llm.NewClient(cfg.AnthropicKey, a.agentCfg.Model)
		if !a.ready {
			a.ready = true
			a.initError = ""
		}
		log.Printf("Settings saved — LLM client reloaded (model=%s)", a.agentCfg.Model)
	}

	// Restart Telegram bot if token is provided, stop if removed.
	if cfg.TelegramToken != "" {
		a.startTelegram(cfg)
	} else {
		a.stopTelegram()
	}

	// Enable or disable push-to-talk dictation.
	if cfg.HotkeyEnabled {
		a.startDictation(cfg)
	} else {
		speech.TeardownDictation()
	}

	return nil
}

// startDictation sets up the global push-to-talk hotkey and pipeline.
func (a *App) startDictation(cfg *config.Config) {
	// Tear down any existing registration first.
	if speech.IsDictationEnabled() {
		speech.TeardownDictation()
	}

	modifier := cfg.HotkeyModifier
	if modifier == "" {
		modifier = speech.DefaultModifier
	}

	speech.SetupDictation(
		modifier,
		a.loadSpeechConfig,
		func(state speech.DictationState, text string) {
			// Track recording start time for duration calculation.
			if state == speech.DictationRecording {
				a.dictStartTime = time.Now()
			}

			// When dictation completes with text, auto-save to flow history.
			if state == speech.DictationIdle && text != "" {
				duration := int(time.Since(a.dictStartTime).Seconds())
				if duration < 1 {
					duration = 1
				}
				go func() {
					id, err := a.SaveFlowTranscript(text, duration)
					if err != nil {
						log.Printf("[dictation] failed to save flow transcript: %v", err)
					} else {
						log.Printf("[dictation] auto-saved flow transcript %s", id)
						// Notify frontend to refresh flow history.
						wailsRuntime.EventsEmit(a.ctx, "flow:saved", map[string]interface{}{
							"id": id,
						})
					}
				}()
			}

			wailsRuntime.EventsEmit(a.ctx, "dictation:status", map[string]interface{}{
				"state": string(state),
				"text":  text,
			})
		},
		func(errMsg string) {
			log.Printf("[dictation] error: %s", errMsg)
			wailsRuntime.EventsEmit(a.ctx, "dictation:error", map[string]interface{}{
				"error": errMsg,
			})
		},
	)
}

// GetDictationStatus returns the current dictation status for the frontend.
func (a *App) GetDictationStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled":       speech.IsDictationEnabled(),
		"state":         string(speech.GetDictationState()),
		"accessibility": speech.HasAccessibilityPermission(false),
	}
}

// RequestAccessibility prompts macOS to show the Accessibility permission dialog.
// Returns true if the app already has permission.
func (a *App) RequestAccessibility() bool {
	return speech.HasAccessibilityPermission(true)
}

// TranscribeAudio takes base64-encoded audio data and transcribes it using
// the configured speech-to-text API (OpenAI Whisper or Deepgram).
// mimeType should be the audio format, e.g. "audio/webm", "audio/wav".
func (a *App) TranscribeAudio(audioBase64 string, mimeType string) (string, error) {
	cfg, err := a.loadSpeechConfig()
	if err != nil {
		return "", err
	}

	log.Printf("[voice] transcribing audio (%s, provider=%s, %d bytes base64)",
		mimeType, cfg.Provider, len(audioBase64))

	result, err := speech.Transcribe(cfg, audioBase64, mimeType)
	if err != nil {
		return "", err
	}

	log.Printf("[voice] transcription complete: %d chars", len(result.Text))
	return result.Text, nil
}

// StartVoiceRecording begins capturing audio from the native macOS microphone.
// The audio is saved to a temporary m4a file. Permission dialogs are handled
// by macOS automatically on first use.
func (a *App) StartVoiceRecording() {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("talon-voice-%d.m4a", time.Now().UnixMilli()))

	a.voiceMu.Lock()
	a.voiceRecordingPath = tmpFile
	a.voiceMu.Unlock()

	log.Printf("[voice] starting native recording → %s", tmpFile)

	speech.StartRecording(tmpFile, func(errMsg string) {
		log.Printf("[voice] recording error: %s", errMsg)
		wailsRuntime.EventsEmit(a.ctx, "voice:error", map[string]interface{}{
			"error": errMsg,
		})
	})
}

// StopVoiceAndTranscribe stops the current recording, sends the audio file
// to the configured speech-to-text API, and returns the transcribed text.
func (a *App) StopVoiceAndTranscribe() (string, error) {
	log.Println("[voice] stopping recording and transcribing...")

	speech.StopRecording()

	// Brief pause to let the file be fully flushed.
	time.Sleep(150 * time.Millisecond)

	a.voiceMu.Lock()
	path := a.voiceRecordingPath
	a.voiceRecordingPath = ""
	a.voiceMu.Unlock()

	if path == "" {
		return "", fmt.Errorf("no recording in progress")
	}

	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read recording file: %w", err)
	}

	if len(data) == 0 {
		return "", fmt.Errorf("recorded file is empty — try speaking louder or closer to the microphone")
	}

	log.Printf("[voice] recorded %d bytes, sending to API...", len(data))

	// Build speech config and transcribe.
	cfg, err := a.loadSpeechConfig()
	if err != nil {
		return "", err
	}

	audioBase64 := base64.StdEncoding.EncodeToString(data)
	result, err := speech.Transcribe(cfg, audioBase64, "audio/m4a")
	if err != nil {
		return "", err
	}

	log.Printf("[voice] transcription complete: %d chars", len(result.Text))
	return result.Text, nil
}

// IsVoiceRecording returns whether a native voice recording is currently active.
func (a *App) IsVoiceRecording() bool {
	return speech.IsRecording()
}

// loadSpeechConfig builds a TranscribeConfig from the current app configuration.
func (a *App) loadSpeechConfig() (speech.TranscribeConfig, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return speech.TranscribeConfig{}, fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return speech.TranscribeConfig{}, fmt.Errorf("load config: %w", err)
	}

	provider := speech.Provider(cfg.SpeechProvider)
	if provider == "" {
		provider = speech.ProviderWhisper
	}

	var apiKey string
	switch provider {
	case speech.ProviderWhisper:
		apiKey = cfg.OpenAIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	case speech.ProviderDeepgram:
		apiKey = cfg.DeepgramKey
		if apiKey == "" {
			apiKey = os.Getenv("DEEPGRAM_API_KEY")
		}
	}

	model := cfg.SpeechModel
	if model == "" {
		model = speech.ModelGPT4oMiniTranscribe
	}

	return speech.TranscribeConfig{
		Provider: provider,
		APIKey:   apiKey,
		Language: "en",
		Model:    model,
	}, nil
}

// StartFlow begins native macOS speech recognition (legacy).
// Results are streamed to the frontend via "flow:result" Wails events.
// Errors are sent via "flow:error" events.
func (a *App) StartFlow(locale string) {
	if locale == "" {
		locale = "en-US"
	}
	log.Printf("[flow] starting speech recognition (locale=%s)", locale)

	speech.Start(locale, func(text string, isFinal bool) {
		wailsRuntime.EventsEmit(a.ctx, "flow:result", map[string]interface{}{
			"text":    text,
			"isFinal": isFinal,
		})
	}, func(errMsg string) {
		log.Printf("[flow] error: %s", errMsg)
		wailsRuntime.EventsEmit(a.ctx, "flow:error", map[string]interface{}{
			"error": errMsg,
		})
	})
}

// StopFlow stops the current speech recognition session (legacy).
func (a *App) StopFlow() {
	log.Println("[flow] stopping speech recognition")
	speech.Stop()
}

// ─── Flow Transcript Storage ───

// FlowTranscriptInfo holds metadata for listing flow transcripts in the sidebar.
type FlowTranscriptInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Duration  int    `json:"duration"`  // recording duration in seconds
	WordCount int    `json:"wordCount"` // number of words
	Timestamp int64  `json:"timestamp"` // unix millis
}

// FlowTranscript is the full saved transcript.
type FlowTranscript struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Duration  int    `json:"duration"`
	WordCount int    `json:"wordCount"`
	Timestamp int64  `json:"timestamp"`
}

// flowDir returns the path to ~/.talon/flow/.
func (a *App) flowDir() (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(baseDir, "flow")
	os.MkdirAll(dir, 0o755)
	return dir, nil
}

// SaveFlowTranscript persists a flow transcript and returns its ID.
func (a *App) SaveFlowTranscript(text string, durationSecs int) (string, error) {
	dir, err := a.flowDir()
	if err != nil {
		return "", err
	}

	ts := time.Now().UnixMilli()
	id := fmt.Sprintf("flow-%d", ts)

	words := len(strings.Fields(text))

	// Build a title from the first 60 chars.
	title := strings.TrimSpace(text)
	if len(title) > 60 {
		title = title[:60] + "..."
	}
	if title == "" {
		title = "Empty recording"
	}

	t := FlowTranscript{
		ID:        id,
		Text:      text,
		Duration:  durationSecs,
		WordCount: words,
		Timestamp: ts,
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal transcript: %w", err)
	}

	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write transcript: %w", err)
	}

	log.Printf("[flow] saved transcript %s (%d words, %ds)", id, words, durationSecs)
	return id, nil
}

// ListFlowTranscripts returns metadata for all saved flow transcripts, newest first.
func (a *App) ListFlowTranscripts() ([]FlowTranscriptInfo, error) {
	dir, err := a.flowDir()
	if err != nil {
		return []FlowTranscriptInfo{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FlowTranscriptInfo{}, nil
		}
		return nil, fmt.Errorf("read flow dir: %w", err)
	}

	var items []FlowTranscriptInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		var t FlowTranscript
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}

		// Build title from text.
		title := strings.TrimSpace(t.Text)
		if len(title) > 60 {
			title = title[:60] + "..."
		}
		if title == "" {
			title = "Empty recording"
		}

		items = append(items, FlowTranscriptInfo{
			ID:        t.ID,
			Title:     title,
			Duration:  t.Duration,
			WordCount: t.WordCount,
			Timestamp: t.Timestamp,
		})
	}

	// Sort newest first.
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Timestamp > items[i].Timestamp {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	return items, nil
}

// LoadFlowTranscript loads a saved flow transcript by ID.
func (a *App) LoadFlowTranscript(id string) (*FlowTranscript, error) {
	dir, err := a.flowDir()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("read transcript %s: %w", id, err)
	}

	var t FlowTranscript
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse transcript %s: %w", id, err)
	}

	return &t, nil
}

// DeleteFlowTranscript removes a saved flow transcript.
func (a *App) DeleteFlowTranscript(id string) error {
	dir, err := a.flowDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete transcript %s: %w", id, err)
	}

	log.Printf("[flow] deleted transcript %s", id)
	return nil
}

// loadDotEnv reads a .env file and sets environment variables.
// Existing environment variables are NOT overridden.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
}
