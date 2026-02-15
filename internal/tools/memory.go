package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- save_memory ---

// SaveMemoryTool persists a key-value pair to the memory directory.
type SaveMemoryTool struct {
	memoryDir string
}

// NewSaveMemoryTool creates a save_memory tool writing to the given directory.
func NewSaveMemoryTool(memoryDir string) *SaveMemoryTool {
	return &SaveMemoryTool{memoryDir: memoryDir}
}

func (t *SaveMemoryTool) Name() string { return "save_memory" }

func (t *SaveMemoryTool) Description() string {
	return "Save a piece of information to persistent memory under a named key. The content is stored as a Markdown file. If the key already exists, the content is overwritten."
}

func (t *SaveMemoryTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "A short, descriptive, lowercase-hyphenated key name (used as filename without extension, e.g. 'user-preferences', 'project-acme-stack')",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The Markdown content to store",
			},
		},
		"required": []string{"key", "content"},
	}
}

type memoryInput struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}

func (t *SaveMemoryTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in memoryInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	if err := os.MkdirAll(t.memoryDir, 0o755); err != nil {
		return fmt.Sprintf("Error creating memory dir: %v", err), nil
	}

	path := filepath.Join(t.memoryDir, in.Key+".md")
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
		return fmt.Sprintf("Error saving memory: %v", err), nil
	}

	return fmt.Sprintf("Memory saved to %s", path), nil
}

// --- memory_search ---

// MemorySearchTool scans memory files for keyword matches in both filenames
// and content. Supports multi-word queries (all words must match).
type MemorySearchTool struct {
	memoryDir string
}

// NewMemorySearchTool creates a memory_search tool reading from the given directory.
func NewMemorySearchTool(memoryDir string) *MemorySearchTool {
	return &MemorySearchTool{memoryDir: memoryDir}
}

func (t *MemorySearchTool) Name() string { return "memory_search" }

func (t *MemorySearchTool) Description() string {
	return "Search persistent memory files for keywords. Searches both filenames and content. Multi-word queries require all words to match. Returns matching file names and content snippets."
}

func (t *MemorySearchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The keyword(s) to search for (case-insensitive). Multiple words are ANDed together.",
			},
		},
		"required": []string{"query"},
	}
}

type searchInput struct {
	Query string `json:"query"`
}

func (t *MemorySearchTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in searchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	pattern := filepath.Join(t.memoryDir, "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Sprintf("Error scanning memory: %v", err), nil
	}

	if len(matches) == 0 {
		return "No memory files found.", nil
	}

	// Split query into words for AND matching.
	queryWords := strings.Fields(strings.ToLower(in.Query))
	if len(queryWords) == 0 {
		return "Empty query.", nil
	}

	var results []string

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		name := filepath.Base(path)
		nameLower := strings.ToLower(name)
		content := string(data)
		contentLower := strings.ToLower(content)

		// Check that all query words appear in either the filename or content.
		searchable := nameLower + " " + contentLower
		allMatch := true
		for _, word := range queryWords {
			if !strings.Contains(searchable, word) {
				allMatch = false
				break
			}
		}

		if !allMatch {
			continue
		}

		// Build the result snippet. For small files, return full content.
		// For larger files, show a context window around the first match.
		const maxSnippet = 2000
		snippet := content
		if len(snippet) > maxSnippet {
			// Find the first match position in content and show context around it.
			idx := strings.Index(contentLower, queryWords[0])
			if idx >= 0 {
				start := idx - 200
				if start < 0 {
					start = 0
				}
				end := idx + len(queryWords[0]) + (maxSnippet - 200)
				if end > len(content) {
					end = len(content)
				}

				snippet = ""
				if start > 0 {
					snippet = "..."
				}
				snippet += content[start:end]
				if end < len(content) {
					snippet += "..."
				}
			} else {
				// Match was in filename only; show beginning of content.
				snippet = content[:maxSnippet] + "..."
			}
		}

		results = append(results, fmt.Sprintf("### %s\n%s", name, snippet))
	}

	if len(results) == 0 {
		return fmt.Sprintf("No memory files matched query %q.", in.Query), nil
	}

	return strings.Join(results, "\n\n---\n\n"), nil
}

// --- list_memories ---

// ListMemoriesTool lists all stored memory files with metadata.
type ListMemoriesTool struct {
	memoryDir string
}

// NewListMemoriesTool creates a list_memories tool for the given directory.
func NewListMemoriesTool(memoryDir string) *ListMemoriesTool {
	return &ListMemoriesTool{memoryDir: memoryDir}
}

func (t *ListMemoriesTool) Name() string { return "list_memories" }

func (t *ListMemoriesTool) Description() string {
	return "List all persistent memory files with their names, sizes, and last-modified dates. Takes no input parameters."
}

func (t *ListMemoriesTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *ListMemoriesTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	pattern := filepath.Join(t.memoryDir, "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Sprintf("Error scanning memory: %v", err), nil
	}

	if len(matches) == 0 {
		return "No memory files found.", nil
	}

	var lines []string
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		size := info.Size()
		modTime := info.ModTime().Format("2006-01-02 15:04:05")

		var sizeStr string
		if size < 1024 {
			sizeStr = fmt.Sprintf("%d bytes", size)
		} else {
			sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
		}

		lines = append(lines, fmt.Sprintf("- **%s** (%s, updated %s)", name, sizeStr, modTime))
	}

	header := fmt.Sprintf("Found %d memory file(s):\n\n", len(lines))
	return header + strings.Join(lines, "\n"), nil
}

// --- delete_memory ---

// DeleteMemoryTool removes a memory file by key.
type DeleteMemoryTool struct {
	memoryDir string
}

// NewDeleteMemoryTool creates a delete_memory tool for the given directory.
func NewDeleteMemoryTool(memoryDir string) *DeleteMemoryTool {
	return &DeleteMemoryTool{memoryDir: memoryDir}
}

func (t *DeleteMemoryTool) Name() string { return "delete_memory" }

func (t *DeleteMemoryTool) Description() string {
	return "Delete a persistent memory file by its key name. Use this to remove outdated or incorrect memories."
}

func (t *DeleteMemoryTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "The key name of the memory to delete (without .md extension)",
			},
		},
		"required": []string{"key"},
	}
}

type deleteInput struct {
	Key string `json:"key"`
}

func (t *DeleteMemoryTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in deleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	path := filepath.Join(t.memoryDir, in.Key+".md")

	// Check if the file exists first.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Sprintf("Memory %q not found.", in.Key), nil
	}

	if err := os.Remove(path); err != nil {
		return fmt.Sprintf("Error deleting memory: %v", err), nil
	}

	return fmt.Sprintf("Memory %q deleted successfully.", in.Key), nil
}
