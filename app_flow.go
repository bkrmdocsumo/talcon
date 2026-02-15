package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/speech"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ─── Flow Transcript Storage ───

// FlowTranscriptInfo holds metadata for listing flow transcripts in the sidebar.
type FlowTranscriptInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Duration  int    `json:"duration"`  // recording duration in seconds
	WordCount int    `json:"wordCount"` // number of words
	Timestamp int64  `json:"timestamp"` // unix millis
}

// FlowTranscript is the full saved transcript.
type FlowTranscript struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Duration  int    `json:"duration"`
	WordCount int    `json:"wordCount"`
	Timestamp int64  `json:"timestamp"`
}

// flowDir returns the path to ~/.talon/flow/.
func (a *App) flowDir() (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(baseDir, "flow")
	os.MkdirAll(dir, 0o755)
	return dir, nil
}

// SaveFlowTranscript persists a flow transcript and returns its ID.
func (a *App) SaveFlowTranscript(text string, durationSecs int) (string, error) {
	dir, err := a.flowDir()
	if err != nil {
		return "", err
	}

	ts := time.Now().UnixMilli()
	id := fmt.Sprintf("flow-%d", ts)

	words := len(strings.Fields(text))

	// Build a title from the first 60 chars.
	title := strings.TrimSpace(text)
	if len(title) > 60 {
		title = title[:60] + "..."
	}
	if title == "" {
		title = "Empty recording"
	}

	t := FlowTranscript{
		ID:        id,
		Text:      text,
		Duration:  durationSecs,
		WordCount: words,
		Timestamp: ts,
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal transcript: %w", err)
	}

	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write transcript: %w", err)
	}

	log.Printf("[flow] saved transcript %s (%d words, %ds)", id, words, durationSecs)
	return id, nil
}

// ListFlowTranscripts returns metadata for all saved flow transcripts, newest first.
func (a *App) ListFlowTranscripts() ([]FlowTranscriptInfo, error) {
	dir, err := a.flowDir()
	if err != nil {
		return []FlowTranscriptInfo{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FlowTranscriptInfo{}, nil
		}
		return nil, fmt.Errorf("read flow dir: %w", err)
	}

	var items []FlowTranscriptInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		var t FlowTranscript
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}

		// Build title from text.
		title := strings.TrimSpace(t.Text)
		if len(title) > 60 {
			title = title[:60] + "..."
		}
		if title == "" {
			title = "Empty recording"
		}

		items = append(items, FlowTranscriptInfo{
			ID:        t.ID,
			Title:     title,
			Duration:  t.Duration,
			WordCount: t.WordCount,
			Timestamp: t.Timestamp,
		})
	}

	// Sort newest first.
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Timestamp > items[i].Timestamp {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	return items, nil
}

// LoadFlowTranscript loads a saved flow transcript by ID.
func (a *App) LoadFlowTranscript(id string) (*FlowTranscript, error) {
	dir, err := a.flowDir()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("read transcript %s: %w", id, err)
	}

	var t FlowTranscript
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse transcript %s: %w", id, err)
	}

	return &t, nil
}

// DeleteFlowTranscript removes a saved flow transcript.
func (a *App) DeleteFlowTranscript(id string) error {
	dir, err := a.flowDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete transcript %s: %w", id, err)
	}

	log.Printf("[flow] deleted transcript %s", id)
	return nil
}

// StartFlow begins native macOS speech recognition (legacy).
// Results are streamed to the frontend via "flow:result" Wails events.
// Errors are sent via "flow:error" events.
func (a *App) StartFlow(locale string) {
	if locale == "" {
		locale = "en-US"
	}
	log.Printf("[flow] starting speech recognition (locale=%s)", locale)

	speech.Start(locale, func(text string, isFinal bool) {
		wailsRuntime.EventsEmit(a.ctx, "flow:result", map[string]interface{}{
			"text":    text,
			"isFinal": isFinal,
		})
	}, func(errMsg string) {
		log.Printf("[flow] error: %s", errMsg)
		wailsRuntime.EventsEmit(a.ctx, "flow:error", map[string]interface{}{
			"error": errMsg,
		})
	})
}

// StopFlow stops the current speech recognition session (legacy).
func (a *App) StopFlow() {
	log.Println("[flow] stopping speech recognition")
	speech.Stop()
}
