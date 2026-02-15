package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/llm"
	"github.com/user/talon/internal/session"
	"github.com/user/talon/internal/tools"
	"github.com/user/talon/internal/util"
)

// maxToolIterations prevents infinite loops in the agent turn.
const maxToolIterations = 25

// agentCodeFileSuffix is appended to the system prompt for agent tasks to
// ensure generated code is always saved as files in the workspace directory.
const agentCodeFileSuffix = `

## Code Output Rules (IMPORTANT)

You MUST always save any code you produce as files using the write_file tool. NEVER just display code in your response without also writing it to a file.

- **Every code snippet** (scripts, configs, HTML, CSS, JSON, YAML, etc.) MUST be saved as a file with an appropriate name and extension.
- If the user asks you to write a program, build something, or generate any code, create the file(s) first using write_file, then explain what you created.
- Use clear, descriptive filenames (e.g. "app.py", "index.html", "schema.sql", "Dockerfile").
- For multi-file projects, organise files in a logical directory structure.
- After writing files, you may still show key parts of the code in your response for explanation, but the file MUST exist.
`

// todoPromptSuffix is appended to the system prompt to instruct the agent
// to use the todo_write tool for task planning. This is injected
// automatically so it works regardless of what the user has in SOUL.md.
const todoPromptSuffix = `

## Task Planning (IMPORTANT)

For any multi-step task, you MUST use the todo_write tool to create a visible plan:

1. **Before doing anything else**, call todo_write with a list of concrete, specific steps (status "pending", first one "in_progress").
2. **As you complete each step**, call todo_write with merge=true to mark it "completed" and the next one "in_progress".
3. **When finished**, ensure all items are "completed".

Keep items short and descriptive (e.g. "Create user database schema", "Add authentication middleware"). This gives the user real-time visibility into your progress.
`

// Step represents a single intermediate step during an agent turn.
type Step struct {
	Type      string `json:"type"`                 // "thinking", "tool_call", "tool_result"
	Content   string `json:"content"`              // thinking text or tool result
	ToolName  string `json:"tool_name,omitempty"`   // for tool_call / tool_result
	ToolInput string `json:"tool_input,omitempty"`  // for tool_call (JSON string)
}

// TurnResult is the structured result of a full agent turn.
type TurnResult struct {
	Steps     []Step `json:"steps"`
	FinalText string `json:"final_text"`
}

// Deps bundles the dependencies required to run an agent turn.
type Deps struct {
	SessionMgr   *session.Manager
	LLMClient    llm.LLMClient
	ToolRegistry *tools.Registry
	BaseDir      string // ~/.talon path
}

// RunAgentTurn executes a full agent turn: sends user input to the LLM,
// handles any tool calls recursively, and returns a structured result with
// thinking steps, tool calls, and the final text response.
// userContent should be a json.RawMessage — either a JSON string (for plain
// text) or a JSON array of Anthropic content blocks (for multimodal messages).
func RunAgentTurn(ctx context.Context, sessionID string, userContent json.RawMessage, agentCfg config.AgentConfig, deps Deps) (*TurnResult, error) {
	// Set session workspace directory in context so file tools
	// write to the workspace. If already set by caller (e.g. agent tasks
	// use ~/.talon/agents/), honour that; otherwise default to sessions/.
	sessionWorkDir := tools.SessionDirFromContext(ctx)
	if sessionWorkDir == "" {
		sessionWorkDir = filepath.Join(deps.BaseDir, "sessions", sessionID)
	}
	ctx = tools.WithSessionDir(ctx, sessionWorkDir)

	// Lock this session to prevent concurrent writes.
	deps.SessionMgr.Lock(sessionID)
	defer deps.SessionMgr.Unlock(sessionID)

	// Load existing conversation history.
	history, err := deps.SessionMgr.Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	// Load system prompt from SOUL file.
	systemPrompt, err := loadSoul(deps.BaseDir, agentCfg.SoulPath)
	if err != nil {
		return nil, fmt.Errorf("load soul: %w", err)
	}

	// Inject memory index into system prompt so the LLM knows what's available.
	if memIdx := buildMemoryIndex(deps.BaseDir); memIdx != "" {
		systemPrompt += memIdx
	}

	// Always inject instructions to save code as files.
	systemPrompt += agentCodeFileSuffix

	// Append the user message.
	userMsg := session.Message{Role: "user", Content: userContent}
	history = append(history, userMsg)
	if err := deps.SessionMgr.Append(sessionID, userMsg); err != nil {
		return nil, fmt.Errorf("append user msg: %w", err)
	}

	// Prepare tool definitions.
	toolDefs := deps.ToolRegistry.AllDefs()

	result := &TurnResult{}

	// Recursive agent loop.
	for i := 0; i < maxToolIterations; i++ {
		resp, err := deps.LLMClient.SendMessages(ctx, systemPrompt, history, toolDefs, agentCfg.EnableThinking)
		if err != nil {
			return nil, fmt.Errorf("llm call: %w", err)
		}

		// Collect thinking steps from this response.
		if thinking := resp.ThinkingContent(); thinking != "" {
			result.Steps = append(result.Steps, Step{
				Type:    "thinking",
				Content: thinking,
			})
		}

		// Persist the assistant response (includes thinking blocks for context continuity).
		assistantMsg := session.Message{
			Role:    "assistant",
			Content: resp.ContentToRaw(),
		}
		history = append(history, assistantMsg)
		if err := deps.SessionMgr.Append(sessionID, assistantMsg); err != nil {
			return nil, fmt.Errorf("append assistant msg: %w", err)
		}

		// If the response is a text-only final answer (no tool use), return it.
		if !resp.HasToolUse() {
			result.FinalText = resp.TextContent()
			return result, nil
		}

		// Process each tool call.
		toolBlocks := resp.ToolUseBlocks()
		for _, tb := range toolBlocks {
			log.Printf("[tool] %s(%s)", tb.Name, string(tb.Input))

			// Record the tool call step.
			result.Steps = append(result.Steps, Step{
				Type:      "tool_call",
				Content:   "",
				ToolName:  tb.Name,
				ToolInput: string(tb.Input),
			})

			// Tool errors are returned as strings (not Go errors) so they
			// can be fed back to the LLM as tool_result content. This lets
			// the model see and reason about tool failures.
			toolResult, err := deps.ToolRegistry.Execute(ctx, tb.Name, tb.Input)
			if err != nil {
				toolResult = fmt.Sprintf("Tool execution error: %v", err)
			}

			log.Printf("[tool] %s -> %d bytes result", tb.Name, len(toolResult))

			// Record the tool result step.
			result.Steps = append(result.Steps, Step{
				Type:     "tool_result",
				Content:  util.TruncateForUI(toolResult, 2000),
				ToolName: tb.Name,
			})

			// Build tool_result content block as per Anthropic spec.
			toolResultContent := []map[string]interface{}{
				{
					"type":        "tool_result",
					"tool_use_id": tb.ID,
					"content":     toolResult,
				},
			}
			resultRaw, err := json.Marshal(toolResultContent)
			if err != nil {
				return nil, fmt.Errorf("marshal tool result: %w", err)
			}

			toolResultMsg := session.Message{
				Role:    "user",
				Content: resultRaw,
			}
			history = append(history, toolResultMsg)
			if err := deps.SessionMgr.Append(sessionID, toolResultMsg); err != nil {
				return nil, fmt.Errorf("append tool result: %w", err)
			}
		}

		// Continue the loop — send tool results back to the LLM.
	}

	return nil, fmt.Errorf("agent exceeded maximum tool iterations (%d)", maxToolIterations)
}

// StreamEvent is a high-level event emitted during a streaming agent turn.
// The Type field determines which frontend event is fired.
type StreamEvent struct {
	Type      string `json:"type"`                 // "thinking_start", "thinking", "text", "tool_call", "tool_result", "todo_update"
	Content   string `json:"content,omitempty"`    // delta text or tool result
	ToolName  string `json:"tool_name,omitempty"`
	ToolInput string `json:"tool_input,omitempty"`
	// TodoItems carries the full todo list snapshot for "todo_update" events.
	TodoItems []tools.TodoItem `json:"todo_items,omitempty"`
}

// RunAgentTurnStream is like RunAgentTurn but streams LLM responses in
// real-time via the emit callback. Text and thinking deltas are emitted as
// they arrive from the API; tool call / result events are emitted after
// each tool execution. The full TurnResult is still returned for
// persistence and final rendering.
func RunAgentTurnStream(ctx context.Context, sessionID string, userContent json.RawMessage, agentCfg config.AgentConfig, deps Deps, emit func(StreamEvent)) (*TurnResult, error) {
	// Set session workspace directory in context so file tools
	// write to the workspace. If already set by caller (e.g. agent tasks
	// use ~/.talon/agents/), honour that; otherwise default to sessions/.
	sessionWorkDir := tools.SessionDirFromContext(ctx)
	if sessionWorkDir == "" {
		sessionWorkDir = filepath.Join(deps.BaseDir, "sessions", sessionID)
	}
	ctx = tools.WithSessionDir(ctx, sessionWorkDir)

	// Lock this session to prevent concurrent writes.
	deps.SessionMgr.Lock(sessionID)
	defer deps.SessionMgr.Unlock(sessionID)

	// Load existing conversation history.
	history, err := deps.SessionMgr.Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	// Load system prompt from SOUL file.
	systemPrompt, err := loadSoul(deps.BaseDir, agentCfg.SoulPath)
	if err != nil {
		return nil, fmt.Errorf("load soul: %w", err)
	}

	// Inject memory index into system prompt so the LLM knows what's available.
	if memIdx := buildMemoryIndex(deps.BaseDir); memIdx != "" {
		systemPrompt += memIdx
	}

	// Inject planning instructions so the agent always uses todo_write.
	systemPrompt += todoPromptSuffix

	// Always inject instructions to save code as files.
	systemPrompt += agentCodeFileSuffix

	// Wire up todo_write callback so the tool can push updates to the frontend.
	ctx = tools.WithTodoCallback(ctx, func(items []tools.TodoItem) {
		emit(StreamEvent{
			Type:      "todo_update",
			TodoItems: items,
		})
	})

	// Append the user message.
	userMsg := session.Message{Role: "user", Content: userContent}
	history = append(history, userMsg)
	if err := deps.SessionMgr.Append(sessionID, userMsg); err != nil {
		return nil, fmt.Errorf("append user msg: %w", err)
	}

	// Prepare tool definitions.
	toolDefs := deps.ToolRegistry.AllDefs()

	result := &TurnResult{}

	// Recursive agent loop.
	for i := 0; i < maxToolIterations; i++ {
		// Use streaming LLM call — thinking and text deltas are emitted in real-time.
		onDelta := func(delta llm.StreamDelta) {
			switch delta.Type {
			case "thinking_start":
				emit(StreamEvent{Type: "thinking_start"})
			case "thinking":
				emit(StreamEvent{Type: "thinking", Content: delta.Content})
			case "text":
				emit(StreamEvent{Type: "text", Content: delta.Content})
			}
		}

		resp, err := deps.LLMClient.SendMessagesStream(ctx, systemPrompt, history, toolDefs, agentCfg.EnableThinking, onDelta)
		if err != nil {
			return nil, fmt.Errorf("llm call: %w", err)
		}

		// Collect thinking steps from this response.
		if thinking := resp.ThinkingContent(); thinking != "" {
			result.Steps = append(result.Steps, Step{
				Type:    "thinking",
				Content: thinking,
			})
		}

		// Persist the assistant response.
		assistantMsg := session.Message{
			Role:    "assistant",
			Content: resp.ContentToRaw(),
		}
		history = append(history, assistantMsg)
		if err := deps.SessionMgr.Append(sessionID, assistantMsg); err != nil {
			return nil, fmt.Errorf("append assistant msg: %w", err)
		}

		// If the response is a text-only final answer (no tool use), return it.
		if !resp.HasToolUse() {
			result.FinalText = resp.TextContent()
			return result, nil
		}

		// Process each tool call.
		toolBlocks := resp.ToolUseBlocks()
		for _, tb := range toolBlocks {
			log.Printf("[tool] %s(%s)", tb.Name, string(tb.Input))

			// Record and emit the tool call step.
			result.Steps = append(result.Steps, Step{
				Type:      "tool_call",
				Content:   "",
				ToolName:  tb.Name,
				ToolInput: string(tb.Input),
			})
			emit(StreamEvent{
				Type:      "tool_call",
				ToolName:  tb.Name,
				ToolInput: string(tb.Input),
			})

			toolResult, err := deps.ToolRegistry.Execute(ctx, tb.Name, tb.Input)
			if err != nil {
				toolResult = fmt.Sprintf("Tool execution error: %v", err)
			}

			log.Printf("[tool] %s -> %d bytes result", tb.Name, len(toolResult))

			truncated := util.TruncateForUI(toolResult, 2000)

			// Record and emit the tool result step.
			result.Steps = append(result.Steps, Step{
				Type:     "tool_result",
				Content:  truncated,
				ToolName: tb.Name,
			})
			emit(StreamEvent{
				Type:     "tool_result",
				Content:  truncated,
				ToolName: tb.Name,
			})

			// Emit file_created events for file-writing tools.
			if tb.Name == "write_file" {
				var fileInput struct {
					Path string `json:"path"`
				}
				if json.Unmarshal(tb.Input, &fileInput) == nil && fileInput.Path != "" {
					// Resolve the full path the same way the tool does.
					resolvedPath := fileInput.Path
					if sessionWorkDir != "" && !filepath.IsAbs(resolvedPath) {
						resolvedPath = filepath.Join(sessionWorkDir, filepath.Clean(resolvedPath))
					}
					fileName := filepath.Base(resolvedPath)
					emit(StreamEvent{
						Type:     "file_created",
						Content:  resolvedPath,
						ToolName: fileName,
					})
				}
			}

			// Build tool_result content block as per Anthropic spec.
			toolResultContent := []map[string]interface{}{
				{
					"type":        "tool_result",
					"tool_use_id": tb.ID,
					"content":     toolResult,
				},
			}
			resultRaw, _ := json.Marshal(toolResultContent)

			toolResultMsg := session.Message{
				Role:    "user",
				Content: resultRaw,
			}
			history = append(history, toolResultMsg)
			if err := deps.SessionMgr.Append(sessionID, toolResultMsg); err != nil {
				return nil, fmt.Errorf("append tool result: %w", err)
			}
		}

		// Continue the loop — send tool results back to the LLM.
	}

	return nil, fmt.Errorf("agent exceeded maximum tool iterations (%d)", maxToolIterations)
}

// loadSoul reads the system prompt file.
func loadSoul(baseDir, soulPath string) (string, error) {
	fullPath := filepath.Join(baseDir, soulPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// buildMemoryIndex scans the memory directory and returns a lightweight index
// of stored memories (name + last-modified date) suitable for injection into
// the system prompt. Returns an empty string if there are no memories.
func buildMemoryIndex(baseDir string) string {
	memDir := filepath.Join(baseDir, "memory")
	pattern := filepath.Join(memDir, "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}

	var lines []string
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		modTime := info.ModTime().Format(time.DateOnly)
		lines = append(lines, fmt.Sprintf("- %s (updated %s)", name, modTime))
	}

	if len(lines) == 0 {
		return ""
	}

	return "\n\n## Available Memories\nThe following memories are stored. Use `memory_search` or `list_memories` to retrieve details.\n" + strings.Join(lines, "\n") + "\n"
}
