package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CronInfo holds metadata about a scheduled cron task.
type CronInfo struct {
	ID       string `json:"id"`
	Schedule string `json:"schedule"`
	Message  string `json:"message"`
}

// CronManager is the interface that the manage_cron tool uses to
// add, list, and remove cron jobs. The concrete implementation is
// provided by the application layer and injected via context.
// AddCron receives the calling context so it can capture the current
// session workspace — cron jobs then run in that same workspace.
type CronManager interface {
	AddCron(ctx context.Context, schedule, message string) (id string, err error)
	ListCrons() ([]CronInfo, error)
	RemoveCron(id string) error
}

type cronManagerKey struct{}

// WithCronManager returns a context that carries a CronManager.
func WithCronManager(ctx context.Context, mgr CronManager) context.Context {
	return context.WithValue(ctx, cronManagerKey{}, mgr)
}

// CronManagerFromContext extracts the CronManager from the context.
func CronManagerFromContext(ctx context.Context) CronManager {
	mgr, _ := ctx.Value(cronManagerKey{}).(CronManager)
	return mgr
}

// ManageCronTool allows the agent to create, list, and remove scheduled
// cron jobs through natural-language conversation.
type ManageCronTool struct{}

func (t *ManageCronTool) Name() string { return "manage_cron" }

func (t *ManageCronTool) Description() string {
	return `Manage scheduled cron jobs that run automatically at specified times. Use this tool to add, list, or remove recurring tasks.

Actions:
- "add": Create a new cron job. Requires "schedule" (cron expression) and "message" (the prompt to send when triggered).
- "list": Show all active cron jobs. No additional parameters needed.
- "remove": Delete a cron job by its ID. Requires "id".

Cron expression format (5 fields): minute hour day-of-month month day-of-week
Examples:
  "0 8 * * *"      → every day at 8:00 AM
  "0 9 * * 1-5"    → weekdays at 9:00 AM
  "30 17 * * *"    → every day at 5:30 PM
  "0 */2 * * *"    → every 2 hours
  "*/15 * * * *"   → every 15 minutes
  "0 8 1 * *"      → 8:00 AM on the 1st of each month`
}

func (t *ManageCronTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"add", "list", "remove"},
				"description": "The action to perform: add, list, or remove a cron job.",
			},
			"schedule": map[string]interface{}{
				"type":        "string",
				"description": "Cron expression (5 fields: minute hour day-of-month month day-of-week). Required for 'add'.",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The prompt/message to send when the cron job triggers. Required for 'add'.",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the cron job to remove. Required for 'remove'.",
			},
		},
		"required": []string{"action"},
	}
}

type manageCronInput struct {
	Action   string `json:"action"`
	Schedule string `json:"schedule"`
	Message  string `json:"message"`
	ID       string `json:"id"`
}

func (t *ManageCronTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in manageCronInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	mgr := CronManagerFromContext(ctx)
	if mgr == nil {
		return "Error: cron management is not available in this mode", nil
	}

	switch in.Action {
	case "add":
		return t.handleAdd(ctx, mgr, in)
	case "list":
		return t.handleList(mgr)
	case "remove":
		return t.handleRemove(mgr, in)
	default:
		return fmt.Sprintf("Error: unknown action %q — must be add, list, or remove", in.Action), nil
	}
}

func (t *ManageCronTool) handleAdd(ctx context.Context, mgr CronManager, in manageCronInput) (string, error) {
	if strings.TrimSpace(in.Schedule) == "" {
		return "Error: 'schedule' is required for the add action", nil
	}
	if strings.TrimSpace(in.Message) == "" {
		return "Error: 'message' is required for the add action", nil
	}

	id, err := mgr.AddCron(ctx, in.Schedule, in.Message)
	if err != nil {
		return fmt.Sprintf("Error adding cron job: %v", err), nil
	}

	return fmt.Sprintf("Cron job created successfully.\n  ID: %s\n  Schedule: %s\n  Message: %s", id, in.Schedule, in.Message), nil
}

func (t *ManageCronTool) handleList(mgr CronManager) (string, error) {
	crons, err := mgr.ListCrons()
	if err != nil {
		return fmt.Sprintf("Error listing cron jobs: %v", err), nil
	}

	if len(crons) == 0 {
		return "No cron jobs are currently scheduled.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Active cron jobs (%d):\n", len(crons))
	for _, c := range crons {
		fmt.Fprintf(&b, "  - ID: %s | Schedule: %s | Message: %s\n", c.ID, c.Schedule, c.Message)
	}
	return b.String(), nil
}

func (t *ManageCronTool) handleRemove(mgr CronManager, in manageCronInput) (string, error) {
	if strings.TrimSpace(in.ID) == "" {
		return "Error: 'id' is required for the remove action", nil
	}

	if err := mgr.RemoveCron(in.ID); err != nil {
		return fmt.Sprintf("Error removing cron job: %v", err), nil
	}

	return fmt.Sprintf("Cron job %q removed successfully.", in.ID), nil
}
