package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/user/talon/internal/config"
)

// ─── Snippets ───

// Snippet holds a trigger → expansion pair for text replacement during dictation.
type Snippet struct {
	ID        string `json:"id"`
	Trigger   string `json:"trigger"`
	Expansion string `json:"expansion"`
	CreatedAt int64  `json:"createdAt"`
}

// snippetsMu protects concurrent reads/writes to the snippets file.
var snippetsMu sync.Mutex

// snippetsFile returns the path to ~/.talon/snippets.json.
func snippetsFile() (string, error) {
	baseDir, err := config.TalonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, "snippets.json"), nil
}

// readSnippets loads all snippets from disk.
func readSnippets() ([]Snippet, error) {
	path, err := snippetsFile()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Snippet{}, nil
		}
		return nil, fmt.Errorf("read snippets: %w", err)
	}

	if len(data) == 0 {
		return []Snippet{}, nil
	}

	var snippets []Snippet
	if err := json.Unmarshal(data, &snippets); err != nil {
		return nil, fmt.Errorf("parse snippets: %w", err)
	}
	return snippets, nil
}

// writeSnippets saves all snippets to disk.
func writeSnippets(snippets []Snippet) error {
	path, err := snippetsFile()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(snippets, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snippets: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write snippets: %w", err)
	}
	return nil
}

// ─── Exported methods (bound to frontend via Wails) ───

// ListSnippets returns all saved snippets.
func (a *App) ListSnippets() ([]Snippet, error) {
	snippetsMu.Lock()
	defer snippetsMu.Unlock()

	return readSnippets()
}

// AddSnippet creates a new snippet and returns it.
func (a *App) AddSnippet(trigger string, expansion string) (Snippet, error) {
	trigger = strings.TrimSpace(trigger)
	expansion = strings.TrimSpace(expansion)
	if trigger == "" {
		return Snippet{}, fmt.Errorf("trigger cannot be empty")
	}
	if expansion == "" {
		return Snippet{}, fmt.Errorf("expansion cannot be empty")
	}

	snippetsMu.Lock()
	defer snippetsMu.Unlock()

	snippets, err := readSnippets()
	if err != nil {
		return Snippet{}, err
	}

	s := Snippet{
		ID:        fmt.Sprintf("snip-%d", time.Now().UnixMilli()),
		Trigger:   trigger,
		Expansion: expansion,
		CreatedAt: time.Now().UnixMilli(),
	}

	snippets = append(snippets, s)

	if err := writeSnippets(snippets); err != nil {
		return Snippet{}, err
	}

	log.Printf("[snippets] added: %q → %q", trigger, expansion)
	return s, nil
}

// UpdateSnippet updates an existing snippet by ID.
func (a *App) UpdateSnippet(id string, trigger string, expansion string) error {
	trigger = strings.TrimSpace(trigger)
	expansion = strings.TrimSpace(expansion)
	if trigger == "" {
		return fmt.Errorf("trigger cannot be empty")
	}
	if expansion == "" {
		return fmt.Errorf("expansion cannot be empty")
	}

	snippetsMu.Lock()
	defer snippetsMu.Unlock()

	snippets, err := readSnippets()
	if err != nil {
		return err
	}

	found := false
	for i, s := range snippets {
		if s.ID == id {
			snippets[i].Trigger = trigger
			snippets[i].Expansion = expansion
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("snippet %s not found", id)
	}

	if err := writeSnippets(snippets); err != nil {
		return err
	}

	log.Printf("[snippets] updated %s: %q → %q", id, trigger, expansion)
	return nil
}

// DeleteSnippet removes a snippet by ID.
func (a *App) DeleteSnippet(id string) error {
	snippetsMu.Lock()
	defer snippetsMu.Unlock()

	snippets, err := readSnippets()
	if err != nil {
		return err
	}

	filtered := make([]Snippet, 0, len(snippets))
	found := false
	for _, s := range snippets {
		if s.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}

	if !found {
		return fmt.Errorf("snippet %s not found", id)
	}

	if err := writeSnippets(filtered); err != nil {
		return err
	}

	log.Printf("[snippets] deleted %s", id)
	return nil
}

// ─── Text replacement ───

// ApplySnippets replaces all snippet triggers in the given text with their
// expansions. Matching is case-insensitive.
func (a *App) ApplySnippets(text string) string {
	snippetsMu.Lock()
	defer snippetsMu.Unlock()

	snippets, err := readSnippets()
	if err != nil {
		log.Printf("[snippets] failed to load for replacement: %v", err)
		return text
	}

	if len(snippets) == 0 {
		return text
	}

	// Apply each snippet as a case-insensitive replacement.
	result := text
	for _, s := range snippets {
		if s.Trigger == "" {
			continue
		}
		result = replaceAllCaseInsensitive(result, s.Trigger, s.Expansion)
	}

	if result != text {
		log.Printf("[snippets] applied replacements: %d chars → %d chars", len(text), len(result))
	}
	return result
}

// replaceAllCaseInsensitive replaces all occurrences of old in s with new,
// matching case-insensitively.
func replaceAllCaseInsensitive(s, old, replacement string) string {
	if old == "" {
		return s
	}

	lower := strings.ToLower(s)
	oldLower := strings.ToLower(old)

	var b strings.Builder
	b.Grow(len(s))

	start := 0
	for {
		idx := strings.Index(lower[start:], oldLower)
		if idx < 0 {
			b.WriteString(s[start:])
			break
		}
		b.WriteString(s[start : start+idx])
		b.WriteString(replacement)
		start += idx + len(old)
	}

	return b.String()
}
