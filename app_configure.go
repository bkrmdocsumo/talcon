package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/talon/internal/config"
)

// ─── Master Prompt ───

// GetMasterPrompt reads the Master_prompt.md file and returns its content.
func (a *App) GetMasterPrompt() (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", fmt.Errorf("resolve talon dir: %w", err)
	}
	promptPath := filepath.Join(baseDir, "workspace", "Master_prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read Master_prompt.md: %w", err)
	}
	return string(data), nil
}

// SaveMasterPrompt writes the given body to Master_prompt.md.
func (a *App) SaveMasterPrompt(body string) error {
	baseDir, err := config.TalonDir()
	if err != nil {
		return fmt.Errorf("resolve talon dir: %w", err)
	}
	promptPath := filepath.Join(baseDir, "workspace", "Master_prompt.md")
	if err := os.WriteFile(promptPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write Master_prompt.md: %w", err)
	}
	log.Printf("[configure] saved Master_prompt.md (%d bytes)", len(body))
	return nil
}

// ─── Memory Files ───

// MemoryFileInfo holds metadata about a memory file.
type MemoryFileInfo struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	UpdatedAt int64  `json:"updatedAt"`
}

// MemoryFileDetail holds the full content of a memory file.
type MemoryFileDetail struct {
	Name      string `json:"name"`
	Body      string `json:"body"`
	Size      int64  `json:"size"`
	UpdatedAt int64  `json:"updatedAt"`
}

func memoryDir() (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "memory"), nil
}

// ListMemoryFiles returns metadata for all .md files in the memory directory.
func (a *App) ListMemoryFiles() ([]MemoryFileInfo, error) {
	dir, err := memoryDir()
	if err != nil {
		return nil, err
	}
	pattern := filepath.Join(dir, "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob memory: %w", err)
	}

	result := make([]MemoryFileInfo, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		result = append(result, MemoryFileInfo{
			Name:      name,
			Size:      info.Size(),
			UpdatedAt: info.ModTime().UnixMilli(),
		})
	}
	return result, nil
}

// GetMemoryFile reads a specific memory file by name and returns its content.
func (a *App) GetMemoryFile(name string) (MemoryFileDetail, error) {
	dir, err := memoryDir()
	if err != nil {
		return MemoryFileDetail{}, err
	}
	path := filepath.Join(dir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return MemoryFileDetail{}, fmt.Errorf("memory %q not found", name)
		}
		return MemoryFileDetail{}, fmt.Errorf("read memory %s: %w", name, err)
	}
	info, _ := os.Stat(path)
	var updatedAt int64
	if info != nil {
		updatedAt = info.ModTime().UnixMilli()
	}
	return MemoryFileDetail{
		Name:      name,
		Body:      string(data),
		Size:      int64(len(data)),
		UpdatedAt: updatedAt,
	}, nil
}

// SaveMemoryFile creates or updates a memory file.
func (a *App) SaveMemoryFile(name string, body string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("memory name cannot be empty")
	}
	dir, err := memoryDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir memory: %w", err)
	}
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write memory %s: %w", name, err)
	}
	log.Printf("[configure] saved memory: %s (%d bytes)", name, len(body))
	return nil
}

// DeleteMemoryFile removes a memory file by name.
func (a *App) DeleteMemoryFile(name string) error {
	dir, err := memoryDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name+".md")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("memory %q not found", name)
		}
		return fmt.Errorf("delete memory %s: %w", name, err)
	}
	log.Printf("[configure] deleted memory: %s", name)
	return nil
}

// AddMemoryFile creates a new memory file with the given name and body.
// Returns metadata about the created file.
func (a *App) AddMemoryFile(name string, body string) (MemoryFileInfo, error) {
	name = sanitizeName(name)
	if name == "" {
		return MemoryFileInfo{}, fmt.Errorf("memory name cannot be empty")
	}
	dir, err := memoryDir()
	if err != nil {
		return MemoryFileInfo{}, err
	}
	path := filepath.Join(dir, name+".md")
	if _, err := os.Stat(path); err == nil {
		return MemoryFileInfo{}, fmt.Errorf("memory %q already exists", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return MemoryFileInfo{}, fmt.Errorf("mkdir memory: %w", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return MemoryFileInfo{}, fmt.Errorf("write memory %s: %w", name, err)
	}
	now := time.Now().UnixMilli()
	log.Printf("[configure] added memory: %s", name)
	return MemoryFileInfo{
		Name:      name,
		Size:      int64(len(body)),
		UpdatedAt: now,
	}, nil
}
