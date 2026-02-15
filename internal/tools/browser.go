package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/user/talon/internal/browser"
)

// --- browser_navigate ---

// BrowserNavigateTool navigates the browser to a URL and returns a semantic snapshot.
type BrowserNavigateTool struct {
	mgr *browser.Manager
}

// NewBrowserNavigateTool creates a browser_navigate tool.
func NewBrowserNavigateTool(mgr *browser.Manager) *BrowserNavigateTool {
	return &BrowserNavigateTool{mgr: mgr}
}

func (t *BrowserNavigateTool) Name() string { return "browser_navigate" }

func (t *BrowserNavigateTool) Description() string {
	return "Navigate the browser to a URL and read its content. This is the primary tool for visiting websites, reading web pages, and fetching online information. Returns a semantic text snapshot of the page showing interactive elements with IDs and page content. Always use this instead of curl/wget."
}

func (t *BrowserNavigateTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to navigate to",
			},
		},
		"required": []string{"url"},
	}
}

type navigateInput struct {
	URL string `json:"url"`
}

func (t *BrowserNavigateTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in navigateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	snapshot, err := t.mgr.Navigate(in.URL)
	if err != nil {
		return fmt.Sprintf("Browser navigation error: %v", err), nil
	}
	return snapshot, nil
}

// --- browser_act ---

// BrowserActTool performs an action on the current page.
type BrowserActTool struct {
	mgr *browser.Manager
}

// NewBrowserActTool creates a browser_act tool.
func NewBrowserActTool(mgr *browser.Manager) *BrowserActTool {
	return &BrowserActTool{mgr: mgr}
}

func (t *BrowserActTool) Name() string { return "browser_act" }

func (t *BrowserActTool) Description() string {
	return "Perform an action on the current browser page. Actions: 'click' (click an element by ID), 'type' (type text into an input by ID), 'scroll' (scroll the page up or down). Returns a fresh semantic snapshot after the action."
}

func (t *BrowserActTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The action to perform: click, type, or scroll",
				"enum":        []string{"click", "type", "scroll"},
			},
			"target_id": map[string]interface{}{
				"type":        "integer",
				"description": "The integer ID of the element to interact with (from the snapshot)",
			},
			"text": map[string]interface{}{
				"type":        "string",
				"description": "Text to type (for 'type' action) or scroll direction 'up'/'down' (for 'scroll' action)",
			},
		},
		"required": []string{"action"},
	}
}

type actInput struct {
	Action   string `json:"action"`
	TargetID int    `json:"target_id"`
	Text     string `json:"text"`
}

func (t *BrowserActTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in actInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	snapshot, err := t.mgr.Act(in.Action, in.TargetID, in.Text)
	if err != nil {
		return fmt.Sprintf("Browser action error: %v", err), nil
	}
	return snapshot, nil
}

// RegisterBrowserTools registers browser_navigate and browser_act tools.
func RegisterBrowserTools(r *Registry, mgr *browser.Manager) {
	r.Register(NewBrowserNavigateTool(mgr))
	r.Register(NewBrowserActTool(mgr))
}
