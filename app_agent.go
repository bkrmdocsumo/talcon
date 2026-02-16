package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/session"
)

// SendAgentTaskStreamWithFiles starts a streaming agent turn for an agent task
// with file attachments. Files are converted to multimodal content blocks using
// the same buildUserContent helper that chat mode uses.
func (a *App) SendAgentTaskStreamWithFiles(input string, files []FileAttachment, sessionID string) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	content := buildUserContent(input, files)

	sid := sessionID
	if sid == "" {
		sid = a.sessionID
	}

	baseDir, err := config.TalonDir()
	if err != nil {
		return fmt.Errorf("resolve talon dir: %w", err)
	}
	workDir := filepath.Join(baseDir, "agents", sid)

	go a.runStream(sid, content, workDir, "agent:stream:event")
	return nil
}

// SendAgentTaskStream starts a streaming agent turn for an agent task.
// The sessionID parameter specifies which session to continue — the frontend
// passes this explicitly to avoid races caused by the shared a.sessionID
// field being overwritten when the user switches between Chat and Agent tabs.
// Files are written to ~/.talon/agents/{session_id}/ instead of sessions/.
// Events are emitted on "agent:stream:event" to avoid mixing with chat streams.
func (a *App) SendAgentTaskStream(input string, sessionID string) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	content, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}

	sid := sessionID
	if sid == "" {
		sid = a.sessionID // fallback for backward compatibility
	}

	baseDir, err := config.TalonDir()
	if err != nil {
		return fmt.Errorf("resolve talon dir: %w", err)
	}
	workDir := filepath.Join(baseDir, "agents", sid)

	go a.runStream(sid, content, workDir, "agent:stream:event")
	return nil
}

// NewAgentSession starts a fresh agent task session and returns the new session ID.
// Agent sessions use "_agent_" in the ID to distinguish them from chat sessions.
func (a *App) NewAgentSession() string {
	a.sessionID = fmt.Sprintf("%s_agent_%d", a.agentCfg.SessionPrefix, time.Now().UnixMilli())
	return a.sessionID
}

// ListAgentSessions returns metadata for all agent task sessions, sorted newest first.
func (a *App) ListAgentSessions() ([]session.SessionInfo, error) {
	if a.deps.SessionMgr == nil {
		return []session.SessionInfo{}, nil
	}
	return a.deps.SessionMgr.ListAgentSessions()
}

// ─── Agent Task Helpers ───

// TaskFileInfo holds metadata about a file in an agent task's working directory.
type TaskFileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// OpenFileInApp opens a file or directory in the default macOS application.
func (a *App) OpenFileInApp(filePath string) error {
	// Expand ~ prefix.
	if strings.HasPrefix(filePath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			filePath = filepath.Join(home, filePath[2:])
		}
	}

	log.Printf("[agent] opening file: %s", filePath)
	cmd := exec.Command("open", filePath)
	return cmd.Run()
}

// RevealInFinder reveals a file in Finder, highlighting it in its parent folder.
func (a *App) RevealInFinder(filePath string) error {
	// Expand ~ prefix.
	if strings.HasPrefix(filePath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			filePath = filepath.Join(home, filePath[2:])
		}
	}

	log.Printf("[agent] revealing in Finder: %s", filePath)
	cmd := exec.Command("open", "-R", filePath)
	return cmd.Run()
}

// GetTaskWorkDir returns the working directory path for an agent task.
// Creates the directory if it doesn't exist. Agent task files live under
// ~/.talon/agents/{taskID}/.
func (a *App) GetTaskWorkDir(taskID string) (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", fmt.Errorf("resolve talon dir: %w", err)
	}

	dir := filepath.Join(baseDir, "agents", taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create task dir: %w", err)
	}

	return dir, nil
}

// ListTaskFiles returns a list of files in an agent task's working directory.
// Agent task files live under ~/.talon/agents/{taskID}/.
func (a *App) ListTaskFiles(taskID string) ([]TaskFileInfo, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return []TaskFileInfo{}, nil
	}

	dir := filepath.Join(baseDir, "agents", taskID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []TaskFileInfo{}, nil
		}
		return nil, fmt.Errorf("read task dir: %w", err)
	}

	var files []TaskFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Skip session metadata files.
		if e.Name() == "session.json" || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		files = append(files, TaskFileInfo{
			Name: e.Name(),
			Path: filepath.Join(dir, e.Name()),
			Size: info.Size(),
		})
	}

	return files, nil
}
