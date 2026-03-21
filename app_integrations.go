package main

import (
	"fmt"
	"log"

	"github.com/user/talon/internal/config"
)

// IntegrationsPayload is the structure exposed to the frontend for
// reading/writing third-party integration credentials.
type IntegrationsPayload struct {
	NotionToken string `json:"notion_token"`
}

// GetIntegrations returns the current integration credentials.
func (a *App) GetIntegrations() (*IntegrationsPayload, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &IntegrationsPayload{
		NotionToken: cfg.NotionToken,
	}, nil
}

// SaveIntegrations persists integration credentials to config.json.
func (a *App) SaveIntegrations(payload IntegrationsPayload) error {
	baseDir, err := config.TalonDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	cfg, err := config.Load(baseDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.NotionToken = payload.NotionToken

	if err := config.Save(baseDir, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	log.Printf("Integrations saved (notion configured: %v)", cfg.NotionToken != "")
	return nil
}
