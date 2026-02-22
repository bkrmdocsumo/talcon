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
	"github.com/user/talon/internal/claw"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/llm"
	"github.com/user/talon/internal/scheduler"
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

	// Stream cancellation — keyed by session ID so concurrent streams
	// (e.g. multiple agent tasks) can be cancelled independently.
	streamMu      sync.Mutex
	streamCancels map[string]context.CancelFunc

	// Voice recording state.
	voiceMu            sync.Mutex
	voiceRecordingPath string

	// Dictation timing (for saving hotkey recordings to flow history).
	dictStartTime time.Time

	// Claw: event-driven gateway system.
	clawGateway   *claw.Gateway
	clawHeartbeat *claw.Heartbeat
	clawScheduler *scheduler.Scheduler
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{
		streamCancels: make(map[string]context.CancelFunc),
	}
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

	// Attempt core initialisation — may fail if no API key is configured yet.
	if err := a.initCore(cfg, baseDir); err != nil {
		a.initError = err.Error()
		log.Printf("startup error: %s", a.initError)
		return
	}

	log.Printf("Talon GUI ready (agent=%s, model=%s)", a.agentCfg.Name, a.agentCfg.Model)

	// Initialise the Claw event gateway.
	a.initClaw(cfg)

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

	// Fire Claw shutdown hook and stop subsystems.
	if a.clawGateway != nil {
		claw.FireShutdownHook(a.clawGateway)
	}
	if a.clawScheduler != nil {
		a.clawScheduler.Stop()
	}
	if a.clawHeartbeat != nil {
		a.clawHeartbeat.Stop()
	}
	if a.clawGateway != nil {
		a.clawGateway.Stop()
	}

	speech.TeardownDictation()
	speech.HideMenuBarIcon()
	a.stopTelegram()
	if a.browserMgr != nil {
		a.browserMgr.Close()
	}
}

// initCore performs the core application initialisation: resolving the agent
// configuration, creating the LLM client, registering tools, and starting the
// browser. It is called from startup() and from SaveSettings() when the app
// was not fully initialised at launch (e.g. missing API key on first run).
func (a *App) initCore(cfg *config.Config, baseDir string) error {
	anthropicKey := cfg.AnthropicKey
	anthropicSrc := "config.json"
	if anthropicKey == "" {
		anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
		anthropicSrc = "env"
	}
	openaiKey := cfg.OpenAIKey
	openaiSrc := "config.json"
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
		openaiSrc = "env"
	}
	geminiKey := cfg.GeminiKey
	geminiSrc := "config.json"
	if geminiKey == "" {
		geminiKey = os.Getenv("GEMINI_API_KEY")
		geminiSrc = "env"
	}

	// Log which API keys were found and their source.
	if anthropicKey != "" {
		log.Printf("Anthropic API key loaded from %s", anthropicSrc)
	}
	if openaiKey != "" {
		log.Printf("OpenAI API key loaded from %s", openaiSrc)
	}
	if geminiKey != "" {
		log.Printf("Gemini API key loaded from %s", geminiSrc)
	}

	// Require at least one provider key to be configured.
	if anthropicKey == "" && openaiKey == "" && geminiKey == "" {
		log.Printf("No API keys found in config.json or environment variables")
		return fmt.Errorf("No API key found. Add at least one API key (Anthropic, OpenAI, or Gemini) in Settings to get started.")
	}

	agentCfg, ok := cfg.Agents["main"]
	if !ok {
		return fmt.Errorf("Agent 'main' not found in config")
	}

	sessionMgr := session.NewManager(baseDir)

	llmClient, err := llm.NewClientForModel(agentCfg.Model, anthropicKey, openaiKey, geminiKey)
	if err != nil {
		// The default model's provider key may not be configured.
		// Fall back to whichever provider has a key available.
		switch {
		case anthropicKey != "":
			llmClient = llm.NewClient(anthropicKey, "claude-sonnet-4-5-20250929")
			agentCfg.Model = "claude-sonnet-4-5-20250929"
		case openaiKey != "":
			llmClient = llm.NewOpenAIClient(openaiKey, "gpt-4o")
			agentCfg.Model = "gpt-4o"
		case geminiKey != "":
			llmClient = llm.NewGeminiClient(geminiKey, "gemini-2.0-flash")
			agentCfg.Model = "gemini-2.0-flash"
		}
		log.Printf("Default model unavailable, fell back to %s", agentCfg.Model)
	}

	toolRegistry := tools.NewRegistry()
	tools.RegisterStandardTools(toolRegistry, baseDir)

	if a.browserMgr == nil {
		browserMgr := browser.NewManager()
		if err := browserMgr.Start(cfg.BrowserHeadless); err != nil {
			log.Printf("Warning: browser failed to start: %v (browser tools disabled)", err)
		} else {
			tools.RegisterBrowserTools(toolRegistry, browserMgr)
			a.browserMgr = browserMgr
			log.Printf("Browser started (headless=%t)", cfg.BrowserHeadless)
		}
	} else {
		tools.RegisterBrowserTools(toolRegistry, a.browserMgr)
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
	a.initError = ""

	return nil
}

// initClaw sets up the Claw event gateway, heartbeat ticker, and fires
// the startup lifecycle hook.
func (a *App) initClaw(cfg *config.Config) {
	emit := func(eventName string, data interface{}) {
		wailsRuntime.EventsEmit(a.ctx, eventName, data)
	}

	a.clawGateway = claw.NewGateway(a.ctx, cfg, a.deps, emit)
	a.clawGateway.SetCronManager(a.cronManager())
	a.clawGateway.Start()

	a.clawHeartbeat = claw.NewHeartbeat(a.clawGateway, claw.DefaultHeartbeatInterval)
	// Heartbeat is not auto-started; user enables it from the Claw dashboard.

	// Start the cron scheduler and load persisted cron tasks.
	a.clawScheduler = scheduler.New(a.ctx)
	a.loadAndRegisterCrons()
	a.clawScheduler.Start()

	// Fire the startup lifecycle hook.
	claw.FireStartupHook(a.clawGateway)

	log.Println("[claw] gateway, scheduler, and hooks initialised")
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
		"ready":              a.ready,
		"agentName":          a.agentCfg.Name,
		"telegramStatus":     tgStatus,
		"userName":           displayName,
		"configuredProviders": a.getConfiguredProviders(),
	}
	if !a.ready {
		result["error"] = a.initError
	}
	return result
}

// getConfiguredProviders returns a list of provider names that have an API key
// configured (in the config file or via environment variables).
func (a *App) getConfiguredProviders() []string {
	var providers []string

	baseDir, err := config.TalonDir()
	if err != nil {
		return providers
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return providers
	}

	anthropicKey := cfg.AnthropicKey
	if anthropicKey == "" {
		anthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if anthropicKey != "" {
		providers = append(providers, "anthropic")
	}

	openaiKey := cfg.OpenAIKey
	if openaiKey == "" {
		openaiKey = os.Getenv("OPENAI_API_KEY")
	}
	if openaiKey != "" {
		providers = append(providers, "openai")
	}

	geminiKey := cfg.GeminiKey
	if geminiKey == "" {
		geminiKey = os.Getenv("GEMINI_API_KEY")
	}
	if geminiKey != "" {
		providers = append(providers, "gemini")
	}

	return providers
}

// OpenLogFile opens the application log file in the default text editor.
func (a *App) OpenLogFile() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home dir: %w", err)
	}
	logPath := filepath.Join(home, ".talon", "talon.log")
	return a.OpenFileInApp(logPath)
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
