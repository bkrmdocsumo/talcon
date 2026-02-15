package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/browser"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/llm"
	"github.com/user/talon/internal/session"
	"github.com/user/talon/internal/speech"
	"github.com/user/talon/internal/tools"
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

	// Resolve OpenAI key for GPT models (from config or env).
	openaiKey := cfg.OpenAIKey
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}

	llmClient, err := llm.NewClientForModel(agentCfg.Model, cfg.AnthropicKey, openaiKey)
	if err != nil {
		// Fall back to Anthropic client — the key check below will catch missing keys.
		llmClient = llm.NewClient(cfg.AnthropicKey, agentCfg.Model)
	}

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
	a.sessionID = fmt.Sprintf("%s_gui_%d", agentCfg.SessionPrefix, time.Now().UnixMilli())
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

// IsReady returns whether the backend has been successfully initialised.
func (a *App) IsReady() bool {
	return a.ready
}

// GetStatus returns the current app status for the frontend to display.
func (a *App) GetStatus() map[string]interface{} {
	a.tgMu.Lock()
	tgStatus := a.tgStatus
	a.tgMu.Unlock()

	// Resolve the display name for the current OS user.
	displayName := ""
	if u, err := user.Current(); err == nil {
		displayName = u.Name
		if displayName == "" {
			displayName = u.Username
		}
		// Use only the first name for a friendlier greeting.
		if parts := strings.Fields(displayName); len(parts) > 0 {
			displayName = parts[0]
		}
	}

	result := map[string]interface{}{
		"ready":          a.ready,
		"agentName":      a.agentCfg.Name,
		"telegramStatus": tgStatus,
		"userName":       displayName,
	}
	if !a.ready {
		result["error"] = a.initError
	}
	return result
}

// GetSessionID returns the current active session ID.
// Returns an empty string if the app hasn't finished starting up yet.
func (a *App) GetSessionID() string {
	if !a.ready {
		return ""
	}
	return a.sessionID
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
