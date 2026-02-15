package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/user/talon/internal/config"
)

const (
	commandTimeout   = 60 * time.Second
	maxOutputBytes   = 10 * 1024 // 10KB
)

// CommandTool executes shell commands with approval checks.
type CommandTool struct {
	approvals *config.ExecApprovals
}

// NewCommandTool creates a run_command tool using the given approval config.
func NewCommandTool(approvals *config.ExecApprovals) *CommandTool {
	return &CommandTool{approvals: approvals}
}

func (t *CommandTool) Name() string { return "run_command" }

func (t *CommandTool) Description() string {
	return "Execute a shell command on the local machine. The command must be in the approval allowlist. Returns combined stdout and stderr, truncated to 10KB. Do NOT use this for fetching web pages — use browser_navigate instead."
}

func (t *CommandTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
		},
		"required": []string{"command"},
	}
}

// commandInput is the expected JSON input for run_command.
type commandInput struct {
	Command string `json:"command"`
}

func (t *CommandTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in commandInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse command input: %w", err)
	}

	if in.Command == "" {
		return "Error: command is empty", nil
	}

	// Check blocked patterns (hard block).
	for _, blocked := range t.approvals.Blocked {
		if strings.Contains(in.Command, blocked) {
			return fmt.Sprintf("Error: command blocked for safety — contains %q", blocked), nil
		}
	}

	// Check allowlist.
	allowed := false
	cmdParts := strings.Fields(in.Command)
	if len(cmdParts) > 0 {
		for _, prefix := range t.approvals.Allowed {
			if cmdParts[0] == prefix || strings.HasPrefix(in.Command, prefix) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return fmt.Sprintf("Error: command %q is not in the approval allowlist. Add it to exec-approvals.json to permit execution.", cmdParts[0]), nil
	}

	// Execute with timeout.
	execCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", in.Command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	output := out.String()
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + "\n... [output truncated at 10KB]"
	}

	if err != nil {
		return fmt.Sprintf("Exit error: %v\n\n%s", err, output), nil
	}

	if output == "" {
		return "(command completed with no output)", nil
	}
	return output, nil
}
