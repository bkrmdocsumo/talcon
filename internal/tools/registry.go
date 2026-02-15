package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/llm"
)

// Tool is the interface that all agent tools must implement.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]interface{}
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// Registry holds all registered tools and provides lookup and serialisation.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get looks up a tool by name. Returns nil if not found.
func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

// Execute runs a named tool with the given input. Returns an error message
// (not a Go error) if the tool is not found, so the LLM can handle it.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t := r.Get(name)
	if t == nil {
		return fmt.Sprintf("Error: unknown tool %q", name), nil
	}
	return t.Execute(ctx, input)
}

// AllDefs returns the tool definitions formatted for the Anthropic API.
func (r *Registry) AllDefs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		})
	}
	return defs
}

// RegisterStandardTools registers all built-in tools (command, file, memory)
// with the given registry. Browser tools are registered separately.
func RegisterStandardTools(r *Registry, baseDir string) {
	// Load exec approvals (use empty if missing).
	approvals, err := config.LoadExecApprovals(baseDir)
	if err != nil {
		approvals = &config.ExecApprovals{}
	}

	r.Register(NewCommandTool(approvals))
	r.Register(&ReadFileTool{})
	r.Register(&WriteFileTool{})

	memoryDir := filepath.Join(baseDir, "memory")
	r.Register(NewSaveMemoryTool(memoryDir))
	r.Register(NewMemorySearchTool(memoryDir))
	r.Register(NewListMemoriesTool(memoryDir))
	r.Register(NewDeleteMemoryTool(memoryDir))

	runtimesDir := filepath.Join(baseDir, "runtimes")
	r.Register(NewSandboxTool(runtimesDir))
}
