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
	"time"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/router"
	"github.com/user/talon/internal/session"
	"github.com/user/talon/internal/tools"
	"github.com/user/talon/internal/util"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// FileAttachment represents a file uploaded by the user in the chat.
type FileAttachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded file content
}

// ChatStep represents a single intermediate step (thinking, tool call, tool result)
// that occurred during an agent turn. Exposed to the frontend.
type ChatStep struct {
	Type      string `json:"type"`                // "thinking", "tool_call", "tool_result"
	Content   string `json:"content"`             // thinking text or tool result
	ToolName  string `json:"tool_name,omitempty"`  // for tool_call / tool_result
	ToolInput string `json:"tool_input,omitempty"` // for tool_call (JSON string)
}

// ChatResponse is the structured response returned to the frontend, containing
// the final text answer along with any intermediate thinking and tool steps.
type ChatResponse struct {
	Steps     []ChatStep `json:"steps"`
	FinalText string     `json:"final_text"`
}

// HistoryMessageFile holds minimal file metadata so the frontend can display
// file attachment chips when loading a saved session.
type HistoryMessageFile struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// HistoryMessage is a simplified message format for loading past sessions
// into the frontend.
type HistoryMessage struct {
	Role    string               `json:"role"`
	Content string               `json:"content"`
	Steps   []ChatStep           `json:"steps"`
	Files   []HistoryMessageFile `json:"files,omitempty"`
}

// SendMessage sends a plain text user message to the agent and returns the response.
func (a *App) SendMessage(input string) (*ChatResponse, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}
	content, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}
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
// The sessionID parameter specifies which session to continue — the frontend
// passes this explicitly to avoid races caused by the shared a.sessionID
// field being overwritten when the user switches between Chat and Agent tabs.
// It returns immediately; results are delivered via "chat:stream:event" Wails events.
func (a *App) SendMessageStream(input string, sessionID string) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	commandBody := a.matchPluginCommand(input)
	content, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}
	sid := sessionID
	if sid == "" {
		sid = a.sessionID // fallback for backward compatibility
	}
	go a.runStream(sid, content, "", "chat:stream:event", commandBody)
	return nil
}

// SendMessageStreamWithFiles starts a streaming agent turn with file attachments.
// The sessionID parameter specifies which session to continue.
// It returns immediately; results are delivered via "chat:stream:event" Wails events.
func (a *App) SendMessageStreamWithFiles(input string, files []FileAttachment, sessionID string) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	commandBody := a.matchPluginCommand(input)
	content := buildUserContent(input, files)
	sid := sessionID
	if sid == "" {
		sid = a.sessionID // fallback for backward compatibility
	}
	go a.runStream(sid, content, "", "chat:stream:event", commandBody)
	return nil
}

// runStream executes a streaming agent turn, emitting events to the frontend
// as content arrives from the LLM. sessionID identifies the conversation so
// events can be routed correctly when multiple streams run concurrently.
// If workspaceDir is non-empty, files are written to that directory instead
// of the default ~/.talon/sessions/{id}/.
// eventName specifies the Wails event channel to emit on (e.g. "chat:stream:event"
// or "agent:stream:event") so that chat and agent streams don't interfere.
func (a *App) runStream(sessionID string, content json.RawMessage, workspaceDir string, eventName string, commandBody ...string) {
	// Create a cancellable child context for this stream.
	streamCtx, cancel := context.WithCancel(a.ctx)
	a.streamMu.Lock()
	a.streamCancels[sessionID] = cancel
	a.streamMu.Unlock()

	defer func() {
		a.streamMu.Lock()
		delete(a.streamCancels, sessionID)
		a.streamMu.Unlock()
		cancel()
	}()

	// Monotonic sequence counter so the frontend can reject duplicate events
	// caused by the macOS WebKit Wails bridge firing the same event twice.
	var seq int64

	emit := func(evt agent.StreamEvent) {
		seq++
		data := map[string]interface{}{
			"session_id": sessionID,
			"seq":        seq,
			"type":       evt.Type,
			"content":    evt.Content,
			"tool_name":  evt.ToolName,
			"tool_input": evt.ToolInput,
		}
		// For file_created events, include path and name fields for the frontend.
		if evt.Type == "file_created" {
			data["path"] = evt.Content  // Path is stored in Content
			data["name"] = evt.ToolName // File name is stored in ToolName
		}
		// For todo_update events, include the full todo items list.
		if evt.Type == "todo_update" && evt.TodoItems != nil {
			items := make([]map[string]interface{}, 0, len(evt.TodoItems))
			for _, item := range evt.TodoItems {
				items = append(items, map[string]interface{}{
					"id":      item.ID,
					"content": item.Content,
					"status":  item.Status,
				})
			}
			data["todo_items"] = items
		}
		wailsRuntime.EventsEmit(a.ctx, eventName, data)
	}

	// If a custom workspace dir is provided (e.g. agent tasks use ~/.talon/agents/),
	// pre-set it in the context so the agent honours it instead of the default.
	if workspaceDir != "" {
		if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
			log.Printf("Warning: failed to create workspace dir %s: %v", workspaceDir, err)
		}
		streamCtx = tools.WithSessionDir(streamCtx, workspaceDir)
	}

	deps := a.deps
	if len(commandBody) > 0 && commandBody[0] != "" {
		deps.CommandBody = commandBody[0]
	}
	// Chat sessions (no custom workspaceDir) get concise replies;
	// agent tasks get the full detailed style.
	deps.ChatMode = (workspaceDir == "")
	result, err := agent.RunAgentTurnStream(streamCtx, sessionID, content, a.agentCfg, deps, emit)
	if err != nil {
		seq++
		wailsRuntime.EventsEmit(a.ctx, eventName, map[string]interface{}{
			"session_id": sessionID,
			"seq":        seq,
			"type":       "error",
			"error":      err.Error(),
		})
		return
	}

	// Emit the final done event with complete response.
	seq++
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
	wailsRuntime.EventsEmit(a.ctx, eventName, map[string]interface{}{
		"session_id": sessionID,
		"seq":        seq,
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
		data, err := json.Marshal(text)
		if err != nil {
			log.Printf("Warning: failed to marshal user text: %v", err)
			return json.RawMessage(`""`)
		}
		return data
	}

	var blocks []map[string]interface{}
	for _, f := range files {
		if util.IsImageMime(f.MimeType) {
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

	data, err := json.Marshal(blocks)
	if err != nil {
		log.Printf("Warning: failed to marshal content blocks: %v", err)
		return json.RawMessage(`""`)
	}
	return data
}


// CancelStream cancels the stream for the given session ID. If sessionID is
// empty, all active streams are cancelled (backward-compatible fallback).
func (a *App) CancelStream(sessionID string) {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	if sessionID != "" {
		if cancel, ok := a.streamCancels[sessionID]; ok {
			log.Printf("Cancelling stream for session %s", sessionID)
			cancel()
			delete(a.streamCancels, sessionID)
		}
		return
	}

	// Fallback: cancel all active streams.
	for sid, cancel := range a.streamCancels {
		log.Printf("Cancelling stream for session %s", sid)
		cancel()
		delete(a.streamCancels, sid)
	}
}

// NewSession starts a fresh conversation (chat) session and returns the new session ID.
func (a *App) NewSession() string {
	a.sessionID = fmt.Sprintf("%s_gui_%d", a.agentCfg.SessionPrefix, time.Now().UnixMilli())
	return a.sessionID
}

// ListSessions returns metadata for all GUI chat sessions, sorted newest first.
func (a *App) ListSessions() ([]session.SessionInfo, error) {
	if a.deps.SessionMgr == nil {
		return []session.SessionInfo{}, nil
	}
	return a.deps.SessionMgr.ListGUISessions()
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

	// Convert to frontend-friendly format, merging consecutive assistant
	// messages into one so the UI displays them as a single response
	// (matching the live streaming behaviour).
	var result []HistoryMessage
	for _, msg := range msgs {
		parsed := parseMessageForFrontend(msg)
		if parsed == nil {
			continue
		}

		// Merge consecutive assistant messages into the previous one.
		if parsed.Role == "assistant" && len(result) > 0 && result[len(result)-1].Role == "assistant" {
			prev := &result[len(result)-1]
			prev.Steps = append(prev.Steps, parsed.Steps...)
			if parsed.Content != "" {
				if prev.Content != "" {
					prev.Content += parsed.Content
				} else {
					prev.Content = parsed.Content
				}
			}
		} else {
			result = append(result, *parsed)
		}
	}

	return result, nil
}

// ListTelegramSessions returns metadata for all Telegram chat sessions, sorted newest first.
func (a *App) ListTelegramSessions() ([]session.SessionInfo, error) {
	if a.deps.SessionMgr == nil {
		return []session.SessionInfo{}, nil
	}
	return a.deps.SessionMgr.ListTelegramSessions()
}

// LoadTelegramSession loads a Telegram session's messages for read-only viewing
// in the frontend. Unlike LoadSession, it does NOT switch the active session.
func (a *App) LoadTelegramSession(sessionID string) ([]HistoryMessage, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}

	msgs, err := a.deps.SessionMgr.Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load telegram session: %w", err)
	}

	var result []HistoryMessage
	for _, msg := range msgs {
		parsed := parseMessageForFrontend(msg)
		if parsed == nil {
			continue
		}

		// Merge consecutive assistant messages.
		if parsed.Role == "assistant" && len(result) > 0 && result[len(result)-1].Role == "assistant" {
			prev := &result[len(result)-1]
			prev.Steps = append(prev.Steps, parsed.Steps...)
			if parsed.Content != "" {
				if prev.Content != "" {
					prev.Content += parsed.Content
				} else {
					prev.Content = parsed.Content
				}
			}
		} else {
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

// ListChatFiles returns a list of files produced in a chat session's workspace.
// Chat session files live under ~/.talon/sessions/{sessionID}/.
func (a *App) ListChatFiles(sessionID string) ([]TaskFileInfo, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return []TaskFileInfo{}, nil
	}

	dir := filepath.Join(baseDir, "sessions", sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskFileInfo{}, nil
		}
		return nil, fmt.Errorf("read session dir: %w", err)
	}

	var files []TaskFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Skip session metadata files (JSONL session logs, dotfiles).
		if strings.HasSuffix(e.Name(), ".jsonl") || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		files = append(files, TaskFileInfo{
			Name: e.Name(),
			Path: filepath.Join(dir, e.Name()),
			Size: info.Size(),
		})
	}

	return files, nil
}

// matchPluginCommand checks if the input starts with a registered plugin
// command and returns the command's markdown body. Returns empty string if no match.
func (a *App) matchPluginCommand(input string) string {
	cmdNames, err := a.ListCommandNames()
	if err != nil || len(cmdNames) == 0 {
		return ""
	}
	matched, _ := router.MatchPluginCommand(input, cmdNames)
	if matched == "" {
		return ""
	}
	body, err := a.GetCommandByName(matched)
	if err != nil {
		log.Printf("[plugins] failed to load command %s: %v", matched, err)
		return ""
	}
	log.Printf("[plugins] matched command /%s", matched)
	return body
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

			// Multimodal user message — extract user text and file metadata
			// separately so the frontend can show file chips without dumping
			// potentially enormous file contents into the message bubble.
			var textParts []string
			var files []HistoryMessageFile
			for _, block := range blocks {
				typ, _ := block["type"].(string)
				switch typ {
				case "text":
					t, _ := block["text"].(string)
					if strings.HasPrefix(t, "File: ") {
						// This is a file content dump created by buildUserContent.
						// Extract just the file name (first line after "File: ").
						name := t[len("File: "):]
						if idx := strings.Index(name, "\n"); idx >= 0 {
							name = name[:idx]
						}
						files = append(files, HistoryMessageFile{Name: name, Type: "text/plain"})
					} else {
						textParts = append(textParts, t)
					}
				case "image":
					// Image attachment — extract media_type for the chip.
					mime := ""
					if src, ok := block["source"].(map[string]interface{}); ok {
						mime, _ = src["media_type"].(string)
					}
					files = append(files, HistoryMessageFile{Name: "image", Type: mime})
				case "document":
					mime := ""
					if src, ok := block["source"].(map[string]interface{}); ok {
						mime, _ = src["media_type"].(string)
					}
					files = append(files, HistoryMessageFile{Name: "document.pdf", Type: mime})
				}
			}
			return &HistoryMessage{
				Role:    "user",
				Content: strings.Join(textParts, "\n"),
				Steps:   []ChatStep{},
				Files:   files,
			}
		}

		// Plain text user message.
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
