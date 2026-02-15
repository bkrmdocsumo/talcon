package router

import (
	"strings"

	"github.com/user/talon/internal/config"
)

// Route inspects the message and returns the appropriate agent configuration.
// Commands prefixed with /research are routed to the "researcher" agent.
// All other messages go to "main".
func Route(message string, cfg *config.Config) config.AgentConfig {
	trimmed := strings.TrimSpace(message)

	// Check for /research command prefix.
	if strings.HasPrefix(trimmed, "/research") {
		if researcher, ok := cfg.Agents["researcher"]; ok {
			return researcher
		}
	}

	// Default to main agent.
	if main, ok := cfg.Agents["main"]; ok {
		return main
	}

	// Fallback: return the first agent found.
	for _, a := range cfg.Agents {
		return a
	}

	// Absolute fallback with sensible defaults.
	return config.AgentConfig{
		Name:          "Talon",
		Model:         "claude-sonnet-4-20250514",
		SoulPath:      "workspace/SOUL.md",
		SessionPrefix: "agent_main",
	}
}

// StripCommand removes a leading /command prefix from the message so the
// agent receives clean input.
func StripCommand(message string) string {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "/research ") {
		return strings.TrimPrefix(trimmed, "/research ")
	}
	return message
}
