package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/session"
	"github.com/user/talon/internal/tools"
	"github.com/user/talon/internal/util"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// FileAttachment represents a file uploaded by the user in the chat.
type FileAttachment struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64-encoded file content
}

// ChatStep represents a single intermediate step (thinking, tool call, tool result)
// that occurred during an agent turn. Exposed to the frontend.
type ChatStep struct {
	Type      string `json:"type"`                // "thinking", "tool_call", "tool_result"
	Content   string `json:"content"`             // thinking text or tool result
	ToolName  string `json:"tool_name,omitempty"`  // for tool_call / tool_result
	ToolInput string `json:"tool_input,omitempty"` // for tool_call (JSON string)
}

// ChatResponse is the structured response returned to the frontend, containing
// the final text answer along with any intermediate thinking and tool steps.
type ChatResponse struct {
	Steps     []ChatStep `json:"steps"`
	FinalText string     `json:"final_text"`
}

// HistoryMessage is a simplified message format for loading past sessions
// into the frontend.
type HistoryMessage struct {
	Role    string     `json:"role"`
	Content string     `json:"content"`
	Steps   []ChatStep `json:"steps"`
}

// SendMessage sends a plain text user message to the agent and returns the response.
func (a *App) SendMessage(input string) (*ChatResponse, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}
	content, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal input: %w", err)
	}
	result, err := agent.RunAgentTurn(a.ctx, a.sessionID, content, a.agentCfg, a.deps)
	if err != nil {
		return nil, err
	}
	return turnResultToChat(result), nil
}

// SendMessageWithFiles sends a user message with optional file attachments to the agent.
// Files are sent as multimodal content blocks to the Anthropic API.
func (a *App) SendMessageWithFiles(input string, files []FileAttachment) (*ChatResponse, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}
	content := buildUserContent(input, files)
	result, err := agent.RunAgentTurn(a.ctx, a.sessionID, content, a.agentCfg, a.deps)
	if err != nil {
		return nil, err
	}
	return turnResultToChat(result), nil
}

// SendMessageStream starts a streaming agent turn for a plain text message.
// It returns immediately; results are delivered via "stream:event" Wails events.
func (a *App) SendMessageStream(input string) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	content, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal input: %w", err)
	}
	go a.runStream(content, "")
	return nil
}

// SendMessageStreamWithFiles starts a streaming agent turn with file attachments.
// It returns immediately; results are delivered via "stream:event" Wails events.
func (a *App) SendMessageStreamWithFiles(input string, files []FileAttachment) error {
	if !a.ready {
		return fmt.Errorf("%s", a.initError)
	}
	content := buildUserContent(input, files)
	go a.runStream(content, "")
	return nil
}

// runStream executes a streaming agent turn, emitting events to the frontend
// as content arrives from the LLM. If workspaceDir is non-empty, files are
// written to that directory instead of the default ~/.talon/sessions/{id}/.
func (a *App) runStream(content json.RawMessage, workspaceDir string) {
	// Create a cancellable child context for this stream.
	streamCtx, cancel := context.WithCancel(a.ctx)
	a.streamMu.Lock()
	a.streamCancel = cancel
	a.streamMu.Unlock()

	defer func() {
		a.streamMu.Lock()
		a.streamCancel = nil
		a.streamMu.Unlock()
		cancel()
	}()

	emit := func(evt agent.StreamEvent) {
		data := map[string]interface{}{
			"type":       evt.Type,
			"content":    evt.Content,
			"tool_name":  evt.ToolName,
			"tool_input": evt.ToolInput,
		}
		// For file_created events, include path and name fields for the frontend.
		if evt.Type == "file_created" {
			data["path"] = evt.Content  // Path is stored in Content
			data["name"] = evt.ToolName // File name is stored in ToolName
		}
		// For todo_update events, include the full todo items list.
		if evt.Type == "todo_update" && evt.TodoItems != nil {
			items := make([]map[string]interface{}, 0, len(evt.TodoItems))
			for _, item := range evt.TodoItems {
				items = append(items, map[string]interface{}{
					"id":      item.ID,
					"content": item.Content,
					"status":  item.Status,
				})
			}
			data["todo_items"] = items
		}
		wailsRuntime.EventsEmit(a.ctx, "stream:event", data)
	}

	// If a custom workspace dir is provided (e.g. agent tasks use ~/.talon/agents/),
	// pre-set it in the context so the agent honours it instead of the default.
	if workspaceDir != "" {
		if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
			log.Printf("Warning: failed to create workspace dir %s: %v", workspaceDir, err)
		}
		streamCtx = tools.WithSessionDir(streamCtx, workspaceDir)
	}

	result, err := agent.RunAgentTurnStream(streamCtx, a.sessionID, content, a.agentCfg, a.deps, emit)
	if err != nil {
		wailsRuntime.EventsEmit(a.ctx, "stream:event", map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	// Emit the final done event with complete response.
	chatResp := turnResultToChat(result)
	stepsData := make([]map[string]interface{}, 0, len(chatResp.Steps))
	for _, s := range chatResp.Steps {
		stepsData = append(stepsData, map[string]interface{}{
			"type":       s.Type,
			"content":    s.Content,
			"tool_name":  s.ToolName,
			"tool_input": s.ToolInput,
		})
	}
	wailsRuntime.EventsEmit(a.ctx, "stream:event", map[string]interface{}{
		"type":       "done",
		"final_text": chatResp.FinalText,
		"steps":      stepsData,
	})
}

// turnResultToChat converts an agent TurnResult to a frontend ChatResponse.
func turnResultToChat(r *agent.TurnResult) *ChatResponse {
	cr := &ChatResponse{FinalText: r.FinalText}
	for _, s := range r.Steps {
		cr.Steps = append(cr.Steps, ChatStep{
			Type:      s.Type,
			Content:   s.Content,
			ToolName:  s.ToolName,
			ToolInput: s.ToolInput,
		})
	}
	return cr
}

// buildUserContent constructs the JSON content for a user message.
// For plain text it returns a JSON string; with files it returns an array
// of Anthropic content blocks (image, document, or text).
func buildUserContent(text string, files []FileAttachment) json.RawMessage {
	if len(files) == 0 {
		data, err := json.Marshal(text)
		if err != nil {
			log.Printf("Warning: failed to marshal user text: %v", err)
			return json.RawMessage(`""`)
		}
		return data
	}

	var blocks []map[string]interface{}
	for _, f := range files {
		if util.IsImageMime(f.MimeType) {
			blocks = append(blocks, map[string]interface{}{
				"type": "image",
				"source": map[string]interface{}{
					"type":       "base64",
					"media_type": f.MimeType,
					"data":       f.Data,
				},
			})
		} else if f.MimeType == "application/pdf" {
			blocks = append(blocks, map[string]interface{}{
				"type": "document",
				"source": map[string]interface{}{
					"type":       "base64",
					"media_type": f.MimeType,
					"data":       f.Data,
				},
			})
		} else {
			// Text-based file — decode from base64 and include as text content.
			decoded, err := base64.StdEncoding.DecodeString(f.Data)
			if err != nil {
				decoded = []byte("[failed to decode file]")
			}
			blocks = append(blocks, map[string]interface{}{
				"type": "text",
				"text": fmt.Sprintf("File: %s\n```\n%s\n```", f.Name, string(decoded)),
			})
		}
	}

	if text != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}

	data, err := json.Marshal(blocks)
	if err != nil {
		log.Printf("Warning: failed to marshal content blocks: %v", err)
		return json.RawMessage(`""`)
	}
	return data
}


// CancelStream cancels the currently running stream, if any.
func (a *App) CancelStream() {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	if a.streamCancel != nil {
		log.Println("Cancelling active stream")
		a.streamCancel()
		a.streamCancel = nil
	}
}

// NewSession starts a fresh conversation (chat) session and returns the new session ID.
func (a *App) NewSession() string {
	a.sessionID = fmt.Sprintf("%s_gui_%d", a.agentCfg.SessionPrefix, time.Now().UnixMilli())
	return a.sessionID
}

// ListSessions returns metadata for all GUI chat sessions, sorted newest first.
func (a *App) ListSessions() ([]session.SessionInfo, error) {
	if a.deps.SessionMgr == nil {
		return []session.SessionInfo{}, nil
	}
	return a.deps.SessionMgr.ListGUISessions()
}

// LoadSession loads a previous session's messages and makes it the active
// session. Returns simplified messages suitable for frontend display.
func (a *App) LoadSession(sessionID string) ([]HistoryMessage, error) {
	if !a.ready {
		return nil, fmt.Errorf("%s", a.initError)
	}

	msgs, err := a.deps.SessionMgr.Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	// Set as active session so subsequent messages continue this conversation.
	a.sessionID = sessionID

	// Convert to frontend-friendly format, merging consecutive assistant
	// messages into one so the UI displays them as a single response
	// (matching the live streaming behaviour).
	var result []HistoryMessage
	for _, msg := range msgs {
		parsed := parseMessageForFrontend(msg)
		if parsed == nil {
			continue
		}

		// Merge consecutive assistant messages into the previous one.
		if parsed.Role == "assistant" && len(result) > 0 && result[len(result)-1].Role == "assistant" {
			prev := &result[len(result)-1]
			prev.Steps = append(prev.Steps, parsed.Steps...)
			if parsed.Content != "" {
				if prev.Content != "" {
					prev.Content += parsed.Content
				} else {
					prev.Content = parsed.Content
				}
			}
		} else {
			result = append(result, *parsed)
		}
	}

	return result, nil
}

// DeleteSession removes a saved session.
func (a *App) DeleteSession(sessionID string) error {
	if a.deps.SessionMgr == nil {
		return fmt.Errorf("app not ready")
	}
	return a.deps.SessionMgr.DeleteSession(sessionID)
}

// parseMessageForFrontend converts a stored session message to a simplified
// HistoryMessage for the frontend. Returns nil for messages that should be
// skipped (e.g. tool_result messages).
func parseMessageForFrontend(msg session.Message) *HistoryMessage {
	if msg.Role == "user" {
		// Check if this is a tool_result message (skip it).
		var blocks []map[string]interface{}
		if json.Unmarshal(msg.Content, &blocks) == nil && len(blocks) > 0 {
			if typ, _ := blocks[0]["type"].(string); typ == "tool_result" {
				return nil
			}
		}
		text := session.ExtractTextFromContent(msg.Content)
		return &HistoryMessage{
			Role:    "user",
			Content: text,
			Steps:   []ChatStep{},
		}
	}

	if msg.Role == "assistant" {
		text := session.ExtractTextFromContent(msg.Content)
		var steps []ChatStep

		// Extract thinking and tool_use steps from content blocks.
		var blocks []map[string]interface{}
		if json.Unmarshal(msg.Content, &blocks) == nil {
			for _, block := range blocks {
				typ, _ := block["type"].(string)
				switch typ {
				case "thinking":
					thinking, _ := block["thinking"].(string)
					steps = append(steps, ChatStep{
						Type:    "thinking",
						Content: thinking,
					})
				case "tool_use":
					name, _ := block["name"].(string)
					inputRaw, _ := json.Marshal(block["input"])
					steps = append(steps, ChatStep{
						Type:      "tool_call",
						ToolName:  name,
						ToolInput: string(inputRaw),
					})
				}
			}
		}

		if steps == nil {
			steps = []ChatStep{}
		}

		return &HistoryMessage{
			Role:    "assistant",
			Content: text,
			Steps:   steps,
		}
	}

	return nil
}
