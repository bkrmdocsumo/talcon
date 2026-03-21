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
		Model:         "claude-sonnet-4-5-20250929",
		PromptPath:    "workspace/Master_prompt.md",
		SessionPrefix: "agent_main",
	}
}

// RouteWithChannelOverride first checks if there's a channel-agent mapping,
// then falls back to text-based routing.
func RouteWithChannelOverride(message string, cfg *config.Config, channelAgent string) config.AgentConfig {
	if channelAgent != "" {
		if agentCfg, ok := cfg.Agents[channelAgent]; ok {
			return agentCfg
		}
	}
	return Route(message, cfg)
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

// MatchPluginCommand checks if the message starts with a registered
// plugin command (e.g. "/brief daily" matches command "brief").
// Returns the matched command name and the remaining message text,
// or empty string if no command matched.
func MatchPluginCommand(message string, commandNames []string) (matched string, rest string) {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, "/") {
		return "", message
	}
	for _, name := range commandNames {
		prefix := "/" + name
		if trimmed == prefix {
			return name, ""
		}
		if strings.HasPrefix(trimmed, prefix+" ") {
			return name, strings.TrimSpace(trimmed[len(prefix)+1:])
		}
	}
	return "", message
}
