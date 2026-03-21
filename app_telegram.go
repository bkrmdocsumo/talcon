package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/gateway"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

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

	// Notifier emits Wails events so the frontend can update the Telegram
	// chat list in real time when messages arrive or replies are sent.
	notify := func(sessionID string, userText string, replyText string) {
		wailsRuntime.EventsEmit(a.ctx, "telegram:activity", map[string]interface{}{
			"session_id": sessionID,
			"user_text":  userText,
			"reply_text": replyText,
		})
	}

	go func() {
		log.Println("[telegram] starting bot from GUI...")
		gateway.RunTelegram(tgCtx, cfg, a.deps, notify, a.accessMgr)
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
