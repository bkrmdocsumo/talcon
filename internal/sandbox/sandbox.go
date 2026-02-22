package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

const (
	defaultTimeout    = 60 * time.Second
	defaultMaxMemMB   = 512
	defaultMaxOutput  = 1024 * 1024 // 1MB
)

// Config controls the sandbox execution environment.
type Config struct {
	RuntimesDir string        // directory containing WASI runtime binaries
	Timeout     time.Duration // max execution time (default 30s)
	MaxMemoryMB uint32        // Wasm memory limit in MB (default 256)
	MaxOutput   int           // max bytes of captured output (default 10KB)
}

func (c Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultTimeout
}

func (c Config) maxOutput() int {
	if c.MaxOutput > 0 {
		return c.MaxOutput
	}
	return defaultMaxOutput
}

// Result holds the output of a sandbox execution.
type Result struct {
	Output   string        // combined stdout + stderr
	ExitCode int           // process exit code (0 = success)
	Duration time.Duration // wall-clock execution time
}

// Run executes the given source code in a WebAssembly sandbox.
// Supported languages: "javascript", "python", "go".
func Run(ctx context.Context, cfg Config, language, code string) (*Result, error) {
	language = strings.ToLower(strings.TrimSpace(language))

	switch language {
	case "javascript", "js":
		return runInterpreted(ctx, cfg, language, code)
	case "python", "py":
		return runInterpreted(ctx, cfg, language, code)
	case "go", "golang":
		return runGo(ctx, cfg, code)
	default:
		return nil, fmt.Errorf("unsupported language: %q (supported: javascript, python, go)", language)
	}
}

// runInterpreted runs code through a WASI-compiled interpreter (QuickJS or CPython).
func runInterpreted(ctx context.Context, cfg Config, language, code string) (*Result, error) {
	// Resolve the WASI runtime binary for this language.
	wasmPath, err := EnsureRuntime(cfg.RuntimesDir, language)
	if err != nil {
		return nil, fmt.Errorf("ensure %s runtime: %w", language, err)
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("read wasm binary: %w", err)
	}

	// Write user code to a temp directory that will be mounted as the guest FS.
	tmpDir, err := os.MkdirTemp("", "talon-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var filename string
	var args []string

	switch language {
	case "javascript", "js":
		filename = "main.js"
		if err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(code), 0o644); err != nil {
			return nil, fmt.Errorf("write code file: %w", err)
		}
		// QuickJS: qjs [filename]
		args = []string{"qjs", filename}
	case "python", "py":
		filename = "main.py"
		if err := os.WriteFile(filepath.Join(tmpDir, filename), []byte(code), 0o644); err != nil {
			return nil, fmt.Errorf("write code file: %w", err)
		}
		// CPython: python [filename]
		args = []string{"python", filename}
	}

	return execWasm(ctx, cfg, wasmBytes, args, tmpDir)
}

// runGo compiles Go source to a WASI binary, then runs it in the sandbox.
func runGo(ctx context.Context, cfg Config, code string) (*Result, error) {
	wasmPath, err := CompileGo(ctx, code, cfg.RuntimesDir)
	if err != nil {
		return nil, fmt.Errorf("compile go: %w", err)
	}
	defer os.Remove(wasmPath)

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("read compiled wasm: %w", err)
	}

	// Create an empty temp dir for the Go program's working directory.
	tmpDir, err := os.MkdirTemp("", "talon-sandbox-go-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	args := []string{"main.wasm"}
	return execWasm(ctx, cfg, wasmBytes, args, tmpDir)
}

// execWasm instantiates and runs a WASI module using wazero.
func execWasm(ctx context.Context, cfg Config, wasmBytes []byte, args []string, workDir string) (*Result, error) {
	// Apply timeout.
	execCtx, cancel := context.WithTimeout(ctx, cfg.timeout())
	defer cancel()

	start := time.Now()

	// Create the wazero runtime.
	rtCfg := wazero.NewRuntimeConfig()
	if cfg.MaxMemoryMB > 0 {
		rtCfg = rtCfg.WithMemoryLimitPages(uint32(cfg.MaxMemoryMB) * 16) // 1 page = 64KB, so MB*16
	} else {
		rtCfg = rtCfg.WithMemoryLimitPages(defaultMaxMemMB * 16)
	}

	rt := wazero.NewRuntimeWithConfig(execCtx, rtCfg)
	defer rt.Close(execCtx)

	// Instantiate WASI host functions.
	wasi_snapshot_preview1.MustInstantiate(execCtx, rt)

	// Capture stdout and stderr.
	var stdout, stderr bytes.Buffer

	// Configure the module: args, stdio, filesystem.
	modCfg := wazero.NewModuleConfig().
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs(args...).
		WithFS(os.DirFS(workDir)).
		WithRandSource(rand.Reader)

	// Instantiate and run the module (calls _start automatically).
	_, err := rt.InstantiateWithConfig(execCtx, wasmBytes, modCfg)

	duration := time.Since(start)

	// Combine stdout + stderr.
	maxOut := cfg.maxOutput()
	output := stdout.String()
	if errOut := stderr.String(); errOut != "" {
		if output != "" {
			output += "\n"
		}
		output += errOut
	}
	if len(output) > maxOut {
		output = output[:maxOut] + "\n... [output truncated at 1MB]"
	}

	// Determine exit code.
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok {
			exitCode = int(exitErr.ExitCode())
			// Exit code 0 is normal termination.
			if exitCode == 0 {
				err = nil
			}
		}
	}

	// If the error is a context timeout, report it clearly.
	if err != nil && execCtx.Err() == context.DeadlineExceeded {
		output += fmt.Sprintf("\n\nExecution timed out after %s", cfg.timeout())
		return &Result{Output: output, ExitCode: 1, Duration: duration}, nil
	}

	// Non-zero exit with output is still a usable result (e.g. runtime error).
	if err != nil && exitCode != 0 {
		return &Result{Output: output, ExitCode: exitCode, Duration: duration}, nil
	}

	// Unexpected error.
	if err != nil {
		return nil, fmt.Errorf("wasm execution failed: %w", err)
	}

	return &Result{Output: output, ExitCode: exitCode, Duration: duration}, nil
}
