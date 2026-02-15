package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/user/talon/internal/sandbox"
)

// SandboxTool executes code in a WebAssembly sandbox.
type SandboxTool struct {
	runtimesDir string
}

// NewSandboxTool creates an execute_code tool that runs code in a Wasm sandbox.
// runtimesDir is the directory where WASI runtime binaries are cached
// (e.g. ~/.talon/runtimes).
func NewSandboxTool(runtimesDir string) *SandboxTool {
	return &SandboxTool{runtimesDir: runtimesDir}
}

func (t *SandboxTool) Name() string { return "execute_code" }

func (t *SandboxTool) Description() string {
	return `Execute code in an isolated WebAssembly sandbox. Supported languages: javascript, python, go. The code runs with no network access, no host filesystem access, and a 30-second timeout. Use this tool when you need to run computations, test algorithms, process data, or verify code correctness. For Go code, the host Go toolchain compiles to WASI before execution.`
}

func (t *SandboxTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"language": map[string]interface{}{
				"type":        "string",
				"description": "The programming language of the code to execute",
				"enum":        []string{"javascript", "python", "go"},
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "The source code to execute. For Go, must be a complete program with package main and func main().",
			},
		},
		"required": []string{"language", "code"},
	}
}

// sandboxInput is the expected JSON input for execute_code.
type sandboxInput struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

func (t *SandboxTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in sandboxInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse sandbox input: %w", err)
	}

	if in.Language == "" {
		return "Error: language is required (javascript, python, or go)", nil
	}
	if in.Code == "" {
		return "Error: code is empty", nil
	}

	cfg := sandbox.Config{
		RuntimesDir: t.runtimesDir,
		Timeout:     30 * time.Second,
		MaxMemoryMB: 256,
		MaxOutput:   10 * 1024,
	}

	result, err := sandbox.Run(ctx, cfg, in.Language, in.Code)
	if err != nil {
		return fmt.Sprintf("Sandbox error: %v", err), nil
	}

	// Format the result for the LLM.
	output := result.Output
	if output == "" {
		output = "(no output)"
	}

	if result.ExitCode != 0 {
		return fmt.Sprintf("Exit code %d (ran for %s):\n\n%s", result.ExitCode, result.Duration.Round(time.Millisecond), output), nil
	}

	return fmt.Sprintf("(%s, %s)\n\n%s", in.Language, result.Duration.Round(time.Millisecond), output), nil
}
