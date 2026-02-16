package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/user/talon/internal/session"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"
const anthropicVersion = "2023-06-01"
const defaultMaxTokens = 8192
const thinkingMaxTokens = 16000
const thinkingBudget = 10000

// ToolDef is the JSON schema definition sent to the Anthropic API.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ThinkingConfig controls extended thinking in the API request.
type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// --- Response types ---

// ContentBlock represents a single block in the response content array.
type ContentBlock struct {
	Type             string          `json:"type"`                          // "text", "tool_use", or "thinking"
	Text             string          `json:"text,omitempty"`                // populated when type == "text"
	Thinking         string          `json:"thinking,omitempty"`            // populated when type == "thinking"
	Signature        string          `json:"signature,omitempty"`           // required for thinking blocks in conversation history
	ID               string          `json:"id,omitempty"`                  // tool_use id
	Name             string          `json:"name,omitempty"`                // tool name
	Input            json.RawMessage `json:"input,omitempty"`               // tool input JSON
	ThoughtSignature string          `json:"thought_signature,omitempty"`   // Gemini: required for function calls with thinking
}

// Response is the parsed Anthropic Messages API response.
type Response struct {
	ID         string         `json:"id"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// HasToolUse returns true if the response contains at least one tool_use block.
func (r *Response) HasToolUse() bool {
	for _, b := range r.Content {
		if b.Type == "tool_use" {
			return true
		}
	}
	return false
}

// TextContent concatenates all text blocks from the response.
func (r *Response) TextContent() string {
	var text string
	for _, b := range r.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	return text
}

// ThinkingContent concatenates all thinking blocks from the response.
func (r *Response) ThinkingContent() string {
	var text string
	for _, b := range r.Content {
		if b.Type == "thinking" {
			text += b.Thinking
		}
	}
	return text
}

// ToolUseBlocks returns only tool_use content blocks.
func (r *Response) ToolUseBlocks() []ContentBlock {
	var blocks []ContentBlock
	for _, b := range r.Content {
		if b.Type == "tool_use" {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// ContentToRaw serialises the response content array into json.RawMessage
// suitable for storing in a session Message.
func (r *Response) ContentToRaw() json.RawMessage {
	data, _ := json.Marshal(r.Content)
	return data
}

// --- Client ---

// Client is the Anthropic Messages API client.
type Client struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewClient creates a new Anthropic API client.
func NewClient(apiKey, model string) *Client {
	return &Client{
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{},
	}
}

// GetModel returns the model identifier.
func (c *Client) GetModel() string {
	return c.Model
}

// apiRequest is the request body for the Messages API.
type apiRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    string            `json:"system,omitempty"`
	Messages  []session.Message `json:"messages"`
	Tools     []ToolDef         `json:"tools,omitempty"`
	Thinking  *ThinkingConfig   `json:"thinking,omitempty"`
	Stream    bool              `json:"stream,omitempty"`
}

// StreamDelta represents a single incremental update during streaming.
type StreamDelta struct {
	Type    string // "thinking_start", "thinking", "text"
	Content string // delta text
}

// SendMessages calls the Anthropic Messages API with the given conversation.
// When enableThinking is true, extended thinking is requested and the response
// may include "thinking" content blocks with the model's internal reasoning.
func (c *Client) SendMessages(ctx context.Context, system string, messages []session.Message, tools []ToolDef, enableThinking bool) (*Response, error) {
	maxTok := defaultMaxTokens
	var thinking *ThinkingConfig
	if enableThinking {
		thinking = &ThinkingConfig{Type: "enabled", BudgetTokens: thinkingBudget}
		maxTok = thinkingMaxTokens
	}

	reqBody := apiRequest{
		Model:     c.Model,
		MaxTokens: maxTok,
		System:    system,
		Messages:  messages,
		Tools:     tools,
		Thinking:  thinking,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	if enableThinking {
		req.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp Response
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &apiResp, nil
}

// SendMessagesStream calls the Anthropic Messages API with streaming enabled.
// As content arrives, onDelta is called with incremental updates. The complete
// assembled Response is returned when the stream finishes, allowing callers
// to process tool calls and persist the response as usual.
func (c *Client) SendMessagesStream(ctx context.Context, system string, messages []session.Message, tools []ToolDef, enableThinking bool, onDelta func(StreamDelta)) (*Response, error) {
	maxTok := defaultMaxTokens
	var thinking *ThinkingConfig
	if enableThinking {
		thinking = &ThinkingConfig{Type: "enabled", BudgetTokens: thinkingBudget}
		maxTok = thinkingMaxTokens
	}

	reqBody := apiRequest{
		Model:     c.Model,
		MaxTokens: maxTok,
		System:    system,
		Messages:  messages,
		Tools:     tools,
		Thinking:  thinking,
		Stream:    true,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	if enableThinking {
		req.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	return parseSSEStream(resp.Body, onDelta)
}

// --- SSE event types for Anthropic streaming ---

type sseContentBlockStart struct {
	Index        int          `json:"index"`
	ContentBlock ContentBlock `json:"content_block"`
}

type sseContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`                   // "text_delta", "thinking_delta", "input_json_delta", "signature_delta"
		Text        string `json:"text,omitempty"`         // for text_delta
		Thinking    string `json:"thinking,omitempty"`     // for thinking_delta
		PartialJSON string `json:"partial_json,omitempty"` // for input_json_delta
		Signature   string `json:"signature,omitempty"`    // for signature_delta
	} `json:"delta"`
}

type sseContentBlockStop struct {
	Index int `json:"index"`
}

type sseMessageStart struct {
	Message struct {
		ID    string `json:"id"`
		Role  string `json:"role"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type sseMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// parseSSEStream reads an Anthropic SSE stream, calls onDelta for text/thinking
// deltas, and returns the fully assembled Response.
func parseSSEStream(body io.Reader, onDelta func(StreamDelta)) (*Response, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var result Response
	var contentBlocks []ContentBlock
	var toolInputBuilders []string // accumulated JSON per content block index
	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		switch currentEvent {
		case "message_start":
			var evt sseMessageStart
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				result.ID = evt.Message.ID
				result.Role = evt.Message.Role
				result.Usage.InputTokens = evt.Message.Usage.InputTokens
			}

		case "content_block_start":
			var evt sseContentBlockStart
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				// Grow slices to accommodate the block index.
				for len(contentBlocks) <= evt.Index {
					contentBlocks = append(contentBlocks, ContentBlock{})
					toolInputBuilders = append(toolInputBuilders, "")
				}
				contentBlocks[evt.Index] = evt.ContentBlock

				if evt.ContentBlock.Type == "thinking" {
					onDelta(StreamDelta{Type: "thinking_start"})
				}
			}

		case "content_block_delta":
			var evt sseContentBlockDelta
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				idx := evt.Index
				if idx < len(contentBlocks) {
					switch evt.Delta.Type {
					case "text_delta":
						contentBlocks[idx].Text += evt.Delta.Text
						onDelta(StreamDelta{Type: "text", Content: evt.Delta.Text})
					case "thinking_delta":
						contentBlocks[idx].Thinking += evt.Delta.Thinking
						onDelta(StreamDelta{Type: "thinking", Content: evt.Delta.Thinking})
					case "input_json_delta":
						toolInputBuilders[idx] += evt.Delta.PartialJSON
					case "signature_delta":
						contentBlocks[idx].Signature += evt.Delta.Signature
					}
				}
			}

		case "content_block_stop":
			var evt sseContentBlockStop
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				idx := evt.Index
				if idx < len(contentBlocks) && contentBlocks[idx].Type == "tool_use" {
					contentBlocks[idx].Input = json.RawMessage(toolInputBuilders[idx])
				}
			}

		case "message_delta":
			var evt sseMessageDelta
			if err := json.Unmarshal([]byte(data), &evt); err == nil {
				result.StopReason = evt.Delta.StopReason
				result.Usage.OutputTokens = evt.Usage.OutputTokens
			}

		case "error":
			// Anthropic may send error events during streaming.
			var apiErr struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &apiErr); err == nil {
				return nil, fmt.Errorf("anthropic stream error (%s): %s", apiErr.Error.Type, apiErr.Error.Message)
			}

		case "message_stop", "ping":
			// No action needed.
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SSE stream: %w", err)
	}

	result.Content = contentBlocks
	return &result, nil
}

// Summarize implements session.Compactor by asking the LLM to summarize a
// conversation. Used for context compaction.
func (c *Client) Summarize(ctx context.Context, system string, messages []session.Message) (string, error) {
	// Append a request to summarize
	summaryRequest, _ := json.Marshal("Please provide a concise summary of the conversation above, preserving all key facts, decisions, and context.")
	allMsgs := append(messages, session.Message{
		Role:    "user",
		Content: summaryRequest,
	})

	resp, err := c.SendMessages(ctx, system, allMsgs, nil, false)
	if err != nil {
		return "", err
	}
	return resp.TextContent(), nil
}
