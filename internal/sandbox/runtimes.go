package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runtime download URLs for pre-compiled WASI binaries.
const (
	// QuickJS-NG compiled to WASI (command model, ~1.5MB).
	quickjsWASMURL = "https://github.com/nicholasgasior/quickjs/releases/download/v0.10.1/qjs-wasi.wasm"
	quickjsFile    = "quickjs.wasm"

	// CPython 3.12.0 compiled to WASI by VMware Labs (~25MB standalone with stdlib bundled).
	pythonWASMURL = "https://github.com/nicholasgasior/webassembly-language-runtimes/releases/download/python%2F3.12.0%2B20231211-040d5a6/python-3.12.0.wasm"
	pythonFile    = "python.wasm"

	downloadTimeout = 5 * time.Minute
)

// runtimeInfo maps a language to its WASI binary filename and download URL.
type runtimeInfo struct {
	filename    string
	downloadURL string
}

var runtimes = map[string]runtimeInfo{
	"javascript": {filename: quickjsFile, downloadURL: quickjsWASMURL},
	"js":         {filename: quickjsFile, downloadURL: quickjsWASMURL},
	"python":     {filename: pythonFile, downloadURL: pythonWASMURL},
	"py":         {filename: pythonFile, downloadURL: pythonWASMURL},
}

// EnsureRuntime checks that the WASI runtime binary for the given language
// exists in runtimesDir. If not, it downloads the binary from its canonical
// source. Returns the absolute path to the .wasm file.
func EnsureRuntime(runtimesDir, language string) (string, error) {
	info, ok := runtimes[strings.ToLower(language)]
	if !ok {
		return "", fmt.Errorf("no WASI runtime available for language %q", language)
	}

	wasmPath := filepath.Join(runtimesDir, info.filename)

	// Already cached?
	if _, err := os.Stat(wasmPath); err == nil {
		return wasmPath, nil
	}

	// Create the runtimes directory.
	if err := os.MkdirAll(runtimesDir, 0o755); err != nil {
		return "", fmt.Errorf("create runtimes dir: %w", err)
	}

	fmt.Printf("[sandbox] Downloading %s runtime (%s)...\n", language, info.downloadURL)

	if err := downloadFile(wasmPath, info.downloadURL); err != nil {
		// Clean up partial download.
		os.Remove(wasmPath)
		return "", fmt.Errorf("download %s runtime: %w", language, err)
	}

	fmt.Printf("[sandbox] Downloaded %s runtime to %s\n", language, wasmPath)
	return wasmPath, nil
}

// downloadFile fetches the given URL and writes it to dest atomically.
func downloadFile(dest, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	// Write to a temp file first, then rename for atomicity.
	tmpFile := dest + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(tmpFile) // clean up if rename failed
	}()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(tmpFile, dest)
}

// CompileGo compiles Go source code to a WASI binary using the host Go
// toolchain (GOOS=wasip1 GOARCH=wasm). Returns the path to the compiled
// .wasm file. The caller is responsible for removing it when done.
func CompileGo(ctx context.Context, code string, outputDir string) (string, error) {
	// Create a temp directory for the Go module.
	tmpDir, err := os.MkdirTemp("", "talon-go-compile-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write go.mod.
	goMod := "module sandbox\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return "", fmt.Errorf("write go.mod: %w", err)
	}

	// Write the user's source code.
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(code), 0o644); err != nil {
		return "", fmt.Errorf("write main.go: %w", err)
	}

	// Compile to WASI.
	outPath := filepath.Join(outputDir, fmt.Sprintf("go-sandbox-%d.wasm", time.Now().UnixNano()))
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", outPath, ".")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build failed:\n%s", stderr.String())
	}

	return outPath, nil
}
