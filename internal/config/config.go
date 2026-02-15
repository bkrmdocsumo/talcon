package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AgentConfig defines the configuration for a single agent persona.
type AgentConfig struct {
	Name           string `json:"name"`
	Model          string `json:"model"`
	SoulPath       string `json:"soul_path"`
	SessionPrefix  string `json:"session_prefix"`
	EnableThinking bool   `json:"enable_thinking"`
}

// Config is the top-level application configuration loaded from config.json.
type Config struct {
	TelegramToken   string                 `json:"telegram_token"`
	AnthropicKey    string                 `json:"anthropic_key"`
	Port            int                    `json:"port"`
	BrowserHeadless bool                   `json:"browser_headless"`
	Agents          map[string]AgentConfig `json:"agents"`

	// Speech-to-text configuration.
	SpeechProvider string `json:"speech_provider"` // "whisper" (default) or "deepgram"
	SpeechModel    string `json:"speech_model"`    // OpenAI model: "gpt-4o-mini-transcribe" (default), "gpt-4o-transcribe", "whisper-1"
	OpenAIKey      string `json:"openai_key"`      // OpenAI API key (for GPT chat models + Whisper/GPT-4o transcribe)
	DeepgramKey    string `json:"deepgram_key"`    // Deepgram API key

	// Global push-to-talk dictation — hold a modifier key to record, release to transcribe & paste.
	HotkeyEnabled  bool   `json:"hotkey_enabled"`  // Enable global push-to-talk hotkey
	HotkeyModifier string `json:"hotkey_modifier"` // "right_option" (default), "left_option", "left_cmd", "right_cmd", "left_ctrl", "right_ctrl"
}

// TalonDir returns the resolved path to ~/.talon/.
func TalonDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".talon"), nil
}

// Bootstrap creates the ~/.talon/ directory tree and default files if they
// do not already exist.
func Bootstrap() (string, error) {
	base, err := TalonDir()
	if err != nil {
		return "", err
	}

	dirs := []string{
		filepath.Join(base, "workspace"),
		filepath.Join(base, "sessions"),
		filepath.Join(base, "memory"),
		filepath.Join(base, "flow"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	// Default config.json
	cfgPath := filepath.Join(base, "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		defaultCfg := Config{
			Port:            8080,
			BrowserHeadless: true,
			Agents: map[string]AgentConfig{
			"main": {
				Name:           "Talon",
				Model:          "claude-sonnet-4-5-20250929",
				SoulPath:       "workspace/SOUL.md",
				SessionPrefix:  "agent_main",
				EnableThinking: true,
			},
			},
		}
		data, _ := json.MarshalIndent(defaultCfg, "", "  ")
		if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
			return "", fmt.Errorf("write config.json: %w", err)
		}
	}

	// Default exec-approvals.json
	approvalsPath := filepath.Join(base, "exec-approvals.json")
	if _, err := os.Stat(approvalsPath); os.IsNotExist(err) {
		defaultApprovals := map[string][]string{
			"allowed": {"ls", "cat", "echo", "pwd", "date", "whoami", "uname", "head", "tail", "wc", "grep", "find", "which", "env", "go"},
			"blocked": {"rm -rf /", "mkfs", "dd if=", ":(){ :|:& };:"},
		}
		data, _ := json.MarshalIndent(defaultApprovals, "", "  ")
		if err := os.WriteFile(approvalsPath, data, 0o644); err != nil {
			return "", fmt.Errorf("write exec-approvals.json: %w", err)
		}
	}

	// Default SOUL.md
	soulPath := filepath.Join(base, "workspace", "SOUL.md")
	if _, err := os.Stat(soulPath); os.IsNotExist(err) {
		soul := `# Talon — System Prompt

You are Talon, a helpful and capable AI assistant running locally on the user's machine.

You have access to tools for running shell commands, reading and writing files, managing persistent memory, and browsing the web.

## Tool Usage Rules

**Web browsing**: When you need to visit a website, read web content, or interact with a web page, you MUST use the ` + "`browser_navigate`" + ` and ` + "`browser_act`" + ` tools. These use a real Chromium browser via Playwright and can render JavaScript, interact with elements, and read full page content. Do NOT use ` + "`run_command`" + ` with curl, wget, or any other HTTP client to fetch web pages — always use the browser tools instead.

**Shell commands** (` + "`run_command`" + `): Only use this for local system tasks (file listing, process management, development commands, etc.). Never use it for fetching web content.

## Memory

You have persistent memory that survives across conversations. Use it proactively:

**When to save memories** (` + "`save_memory`" + `):
- User preferences, habits, or personal details they share (name, location, preferences)
- Project context: names, tech stacks, directory structures, key decisions
- Important facts or instructions the user wants you to remember
- Outcomes of complex tasks for future reference

**When to search memories** (` + "`memory_search`" + `):
- At the start of a conversation if the user references something that might have prior context
- Before answering questions that might relate to previously stored information
- When the user says "remember", "recall", "we discussed", or similar phrases

**When to list memories** (` + "`list_memories`" + `):
- When you need an overview of what you already know
- When the user asks what you remember about them

**When to delete memories** (` + "`delete_memory`" + `):
- When a user corrects previously saved information
- When information is outdated or no longer relevant
- When the user asks you to forget something

**Key naming conventions**: Use lowercase, hyphenated, descriptive keys (e.g., ` + "`user-preferences`" + `, ` + "`project-acme-stack`" + `, ` + "`meeting-2026-02-14`" + `). Group related information under the same key rather than creating many small memories.

## Task Planning

When working on a multi-step task, **always** start by creating a plan using the ` + "`todo_write`" + ` tool:

1. **At the start of a task**: Call ` + "`todo_write`" + ` with a list of concrete steps you plan to take (all with status "pending", or the first one as "in_progress").
2. **As you work**: Call ` + "`todo_write`" + ` with ` + "`merge: true`" + ` to update step statuses — mark the current step as "in_progress" and completed ones as "completed".
3. **When done**: Ensure all steps are marked "completed".

Keep plan items concise and specific (e.g. "Create database schema for users table", not "Do the database stuff"). This gives the user real-time visibility into your progress.

## Behaviour

Be concise but thorough. When using tools, explain what you are doing and why. If a tool fails, analyse the error and retry with a corrected approach.
`
		if err := os.WriteFile(soulPath, []byte(soul), 0o644); err != nil {
			return "", fmt.Errorf("write SOUL.md: %w", err)
		}
	}

	return base, nil
}

// Save writes the configuration back to config.json in the given base directory.
func Save(base string, cfg *Config) error {
	cfgPath := filepath.Join(base, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return fmt.Errorf("write config.json: %w", err)
	}
	return nil
}

// Load reads config.json from the given base directory.
func Load(base string) (*Config, error) {
	cfgPath := filepath.Join(base, "config.json")
	f, err := os.Open(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("open config.json: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config.json: %w", err)
	}

	// Ensure sensible defaults
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentConfig)
	}

	return &cfg, nil
}

// ExecApprovals holds allowlisted and blocklisted command prefixes.
type ExecApprovals struct {
	Allowed []string `json:"allowed"`
	Blocked []string `json:"blocked"`
}

// LoadExecApprovals reads exec-approvals.json from the base directory.
func LoadExecApprovals(base string) (*ExecApprovals, error) {
	path := filepath.Join(base, "exec-approvals.json")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open exec-approvals.json: %w", err)
	}
	defer f.Close()

	var approvals ExecApprovals
	if err := json.NewDecoder(f).Decode(&approvals); err != nil {
		return nil, fmt.Errorf("decode exec-approvals.json: %w", err)
	}
	return &approvals, nil
}
