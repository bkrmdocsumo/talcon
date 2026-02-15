package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/browser"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/gateway"
	"github.com/user/talon/internal/llm"
	"github.com/user/talon/internal/scheduler"
	"github.com/user/talon/internal/session"
	"github.com/user/talon/internal/tools"
)

func main() {
	mode := flag.String("mode", "cli", "Run mode: cli, server")
	agentName := flag.String("agent", "main", "Agent to use from config")
	noBrowser := flag.Bool("no-browser", false, "Disable browser tools")
	flag.Parse()

	// Bootstrap ~/.talon/ directory and defaults.
	baseDir, err := config.Bootstrap()
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	log.Printf("Talon home: %s", baseDir)

	// Load configuration.
	cfg, err := config.Load(baseDir)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if cfg.AnthropicKey == "" {
		cfg.AnthropicKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if cfg.AnthropicKey == "" {
		log.Fatal("No Anthropic API key. Set 'anthropic_key' in config.json or ANTHROPIC_API_KEY env var.")
	}

	// Also allow Telegram token from env var.
	if cfg.TelegramToken == "" {
		cfg.TelegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	}

	// Resolve agent config.
	agentCfg, ok := cfg.Agents[*agentName]
	if !ok {
		log.Fatalf("agent %q not found in config", *agentName)
	}

	// Initialise core components.
	sessionMgr := session.NewManager(baseDir)
	llmClient := llm.NewClient(cfg.AnthropicKey, agentCfg.Model)
	toolRegistry := tools.NewRegistry()

	// Register standard tools (command, file, memory).
	tools.RegisterStandardTools(toolRegistry, baseDir)

	// Optionally start browser and register browser tools.
	var browserMgr *browser.Manager
	if !*noBrowser {
		browserMgr = browser.NewManager()
		if err := browserMgr.Start(cfg.BrowserHeadless); err != nil {
			log.Printf("Warning: browser failed to start: %v (browser tools disabled)", err)
			browserMgr = nil
		} else {
			tools.RegisterBrowserTools(toolRegistry, browserMgr)
			defer browserMgr.Close()
			log.Printf("Browser started (headless=%t)", cfg.BrowserHeadless)
		}
	}

	deps := agent.Deps{
		SessionMgr:   sessionMgr,
		LLMClient:    llmClient,
		ToolRegistry: toolRegistry,
		BaseDir:      baseDir,
	}

	// Context with signal-based cancellation.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start cron scheduler with example morning briefing.
	sched := scheduler.New(ctx)
	sched.AddTask(scheduler.Task{
		Schedule:  "0 8 * * *",
		SessionID: "cron:morning",
		AgentName: "main",
		Message:   "Good morning! Please provide a brief status update and any reminders for today.",
	}, cfg, deps)
	sched.Start()
	defer sched.Stop()

	switch *mode {
	case "cli":
		if err := gateway.RunCLI(ctx, agentCfg, deps); err != nil {
			log.Fatalf("cli: %v", err)
		}

	case "server":
		// Start HTTP gateway.
		go gateway.RunHTTP(ctx, cfg, deps)

		// Start Telegram if configured.
		if cfg.TelegramToken != "" {
			go gateway.RunTelegram(ctx, cfg, deps)
		}

		log.Printf("Server running on :%d (Ctrl+C to stop)", cfg.Port)

		// Block until shutdown signal.
		<-ctx.Done()
		log.Println("Shutting down...")

	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		os.Exit(1)
	}
}
