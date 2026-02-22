package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/user/talon/internal/claw"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/scheduler"
	"github.com/user/talon/internal/tools"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ClawEventInfo is the frontend-friendly representation of a processed event.
type ClawEventInfo struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Source      string `json:"source"`
	Payload     string `json:"payload"`
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`
	Result      string `json:"result"`
	Timestamp   string `json:"timestamp"`
	ProcessedAt string `json:"processed_at"`
	Hidden      bool   `json:"hidden"`
}

// ClawConfig holds the user-facing Claw trigger configuration.
type ClawConfig struct {
	HeartbeatEnabled  bool `json:"heartbeat_enabled"`
	HeartbeatInterval int  `json:"heartbeat_interval_minutes"`
	HooksEnabled      bool `json:"hooks_enabled"`
	WebhooksEnabled   bool `json:"webhooks_enabled"`
	CronsEnabled      bool `json:"crons_enabled"`
}

// ClawStats holds dashboard summary statistics.
type ClawStats struct {
	TotalEvents   int               `json:"total_events"`
	EventsToday   int               `json:"events_today"`
	LastHeartbeat string            `json:"last_heartbeat"`
	CountsByType  map[string]int    `json:"counts_by_type"`
	GatewayActive bool              `json:"gateway_active"`
}

// GetClawEvents returns the recent event history for the Claw dashboard.
func (a *App) GetClawEvents() []ClawEventInfo {
	if a.clawGateway == nil {
		return []ClawEventInfo{}
	}

	records := a.clawGateway.History()
	out := make([]ClawEventInfo, 0, len(records))
	for _, rec := range records {
		out = append(out, ClawEventInfo{
			ID:          rec.Event.ID,
			Type:        string(rec.Event.Type),
			Source:      rec.Event.Source,
			Payload:     rec.Event.Payload,
			SessionID:   rec.Event.SessionID,
			Status:      string(rec.Status),
			Result:      truncateStr(rec.Result, 500),
			Timestamp:   rec.Event.Timestamp.Format(time.RFC3339),
			ProcessedAt: rec.ProcessedAt.Format(time.RFC3339),
			Hidden:      rec.Event.Hidden,
		})
	}
	return out
}

// GetClawConfig returns the current trigger configuration.
func (a *App) GetClawConfig() ClawConfig {
	cfg := ClawConfig{
		HooksEnabled:    true,
		WebhooksEnabled: true,
		CronsEnabled:    true,
	}
	if a.clawHeartbeat != nil {
		cfg.HeartbeatEnabled = a.clawHeartbeat.IsRunning()
		cfg.HeartbeatInterval = int(a.clawHeartbeat.Interval().Minutes())
	}
	return cfg
}

// SetClawConfig updates the Claw trigger settings.
func (a *App) SetClawConfig(cfg ClawConfig) {
	if a.clawHeartbeat == nil {
		return
	}

	if cfg.HeartbeatEnabled && !a.clawHeartbeat.IsRunning() {
		if cfg.HeartbeatInterval > 0 {
			a.clawHeartbeat.SetInterval(time.Duration(cfg.HeartbeatInterval) * time.Minute)
		}
		a.clawHeartbeat.Start()
	} else if !cfg.HeartbeatEnabled && a.clawHeartbeat.IsRunning() {
		a.clawHeartbeat.Stop()
	}
}

// PushManualEvent lets the user manually push an event from the Claw dashboard.
func (a *App) PushManualEvent(eventType string, message string) string {
	if a.clawGateway == nil {
		return ""
	}

	var evtType claw.EventType
	switch eventType {
	case "heartbeat":
		evtType = claw.EventHeartbeat
	case "hook":
		evtType = claw.EventHook
	case "message":
		evtType = claw.EventMessage
	default:
		evtType = claw.EventMessage
	}

	evt := claw.NewEvent(evtType, message, "manual:ui", "claw:manual", "main")
	if evtType == claw.EventHeartbeat {
		evt.Hidden = true
	}
	a.clawGateway.Push(evt)
	return evt.ID
}

// GetClawStats returns aggregate stats for the dashboard.
func (a *App) GetClawStats() ClawStats {
	stats := ClawStats{
		CountsByType:  map[string]int{},
		GatewayActive: a.clawGateway != nil,
	}
	if a.clawGateway == nil {
		return stats
	}

	raw := a.clawGateway.Stats()
	if v, ok := raw["total"].(int); ok {
		stats.TotalEvents = v
	}
	if v, ok := raw["today"].(int); ok {
		stats.EventsToday = v
	}
	if v, ok := raw["last_heartbeat"].(time.Time); ok && !v.IsZero() {
		stats.LastHeartbeat = v.Format(time.RFC3339)
	}
	if v, ok := raw["counts_by_type"].(map[claw.EventType]int); ok {
		for k, c := range v {
			stats.CountsByType[string(k)] = c
		}
	}
	return stats
}

// FireHeartbeatNow triggers a heartbeat event immediately (for testing).
func (a *App) FireHeartbeatNow() {
	if a.clawHeartbeat != nil {
		a.clawHeartbeat.FireNow()
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ─── Cron Task Management ───

// CronTaskInfo is the frontend-friendly representation of a scheduled cron task.
type CronTaskInfo struct {
	ID           string `json:"id"`
	Schedule     string `json:"schedule"`
	Message      string `json:"message"`
	WorkspaceDir string `json:"workspace_dir,omitempty"`
}

// cronsPersistFile returns the path to ~/.talon/claw_crons.json.
func cronsPersistFile() (string, error) {
	dir, err := config.TalonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "claw_crons.json"), nil
}

func loadCronTasks() ([]CronTaskInfo, error) {
	path, err := cronsPersistFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tasks []CronTaskInfo
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func saveCronTasks(tasks []CronTaskInfo) error {
	path, err := cronsPersistFile()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// loadAndRegisterCrons reads persisted cron tasks and registers them with the scheduler.
func (a *App) loadAndRegisterCrons() {
	tasks, err := loadCronTasks()
	if err != nil {
		log.Printf("[claw] failed to load persisted crons: %v", err)
		return
	}
	for _, t := range tasks {
		task := scheduler.Task{
			Schedule:     t.Schedule,
			SessionID:    "cron:" + t.ID,
			AgentName:    "main",
			Message:      t.Message,
			WorkspaceDir: t.WorkspaceDir,
		}
		if err := a.clawScheduler.AddTaskWithGateway(task, a.clawGateway); err != nil {
			log.Printf("[claw] failed to register cron %q: %v", t.ID, err)
		} else {
			log.Printf("[claw] registered cron %q (%s): %s", t.ID, t.Schedule, t.Message)
		}
	}
}

// AddClawCron validates the cron expression, registers it with the scheduler,
// and persists it to disk.
func (a *App) AddClawCron(schedule string, message string) error {
	if a.clawScheduler == nil || a.clawGateway == nil {
		return fmt.Errorf("claw gateway not initialised")
	}

	id := uuid.New().String()[:8]

	task := scheduler.Task{
		Schedule:  schedule,
		SessionID: "cron:" + id,
		AgentName: "main",
		Message:   message,
	}

	if err := a.clawScheduler.AddTaskWithGateway(task, a.clawGateway); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	tasks, _ := loadCronTasks()
	tasks = append(tasks, CronTaskInfo{
		ID:       id,
		Schedule: schedule,
		Message:  message,
	})
	if err := saveCronTasks(tasks); err != nil {
		log.Printf("[claw] failed to persist cron: %v", err)
	}

	log.Printf("[claw] added cron %q (%s): %s", id, schedule, message)
	return nil
}

// ListClawCrons returns all persisted cron tasks.
func (a *App) ListClawCrons() []CronTaskInfo {
	tasks, err := loadCronTasks()
	if err != nil {
		log.Printf("[claw] failed to load crons: %v", err)
		return []CronTaskInfo{}
	}
	if tasks == nil {
		return []CronTaskInfo{}
	}
	return tasks
}

// RemoveClawCron removes a cron task by ID. Since robfig/cron doesn't support
// removing individual entries without tracking entry IDs, we rebuild the
// scheduler with the remaining tasks.
func (a *App) RemoveClawCron(id string) error {
	tasks, err := loadCronTasks()
	if err != nil {
		return err
	}

	var remaining []CronTaskInfo
	found := false
	for _, t := range tasks {
		if t.ID == id {
			found = true
			continue
		}
		remaining = append(remaining, t)
	}
	if !found {
		return fmt.Errorf("cron task %q not found", id)
	}

	if err := saveCronTasks(remaining); err != nil {
		return err
	}

	// Rebuild the scheduler: stop the old one, create a new one, re-register remaining tasks.
	a.clawScheduler.Stop()
	a.clawScheduler = scheduler.New(a.ctx)
	for _, t := range remaining {
		task := scheduler.Task{
			Schedule:     t.Schedule,
			SessionID:    "cron:" + t.ID,
			AgentName:    "main",
			Message:      t.Message,
			WorkspaceDir: t.WorkspaceDir,
		}
		if err := a.clawScheduler.AddTaskWithGateway(task, a.clawGateway); err != nil {
			log.Printf("[claw] failed to re-register cron %q: %v", t.ID, err)
		}
	}
	a.clawScheduler.Start()

	log.Printf("[claw] removed cron %q, %d remaining", id, len(remaining))
	return nil
}

// ─── CronManager adapter (for the manage_cron tool) ───

// appCronManager adapts the App's Wails-bound cron methods to the
// tools.CronManager interface so the agent can manage crons via tool calls.
type appCronManager struct {
	app *App
}

func (m *appCronManager) AddCron(ctx context.Context, schedule, message string) (string, error) {
	if m.app.clawScheduler == nil || m.app.clawGateway == nil {
		return "", fmt.Errorf("claw gateway not initialised")
	}

	id := uuid.New().String()[:8]
	workspaceDir := tools.SessionDirFromContext(ctx)

	task := scheduler.Task{
		Schedule:     schedule,
		SessionID:    "cron:" + id,
		AgentName:    "main",
		Message:      message,
		WorkspaceDir: workspaceDir,
	}

	if err := m.app.clawScheduler.AddTaskWithGateway(task, m.app.clawGateway); err != nil {
		return "", fmt.Errorf("invalid cron expression: %w", err)
	}

	tasks, _ := loadCronTasks()
	tasks = append(tasks, CronTaskInfo{
		ID:           id,
		Schedule:     schedule,
		Message:      message,
		WorkspaceDir: workspaceDir,
	})
	if err := saveCronTasks(tasks); err != nil {
		log.Printf("[claw] failed to persist cron: %v", err)
	}

	log.Printf("[claw] added cron %q (%s): %s (workspace=%s)", id, schedule, message, workspaceDir)
	m.app.emitCronsUpdated()
	return id, nil
}

func (m *appCronManager) ListCrons() ([]tools.CronInfo, error) {
	tasks := m.app.ListClawCrons()
	out := make([]tools.CronInfo, len(tasks))
	for i, t := range tasks {
		out[i] = tools.CronInfo{
			ID:       t.ID,
			Schedule: t.Schedule,
			Message:  t.Message,
		}
	}
	return out, nil
}

func (m *appCronManager) RemoveCron(id string) error {
	if err := m.app.RemoveClawCron(id); err != nil {
		return err
	}
	m.app.emitCronsUpdated()
	return nil
}

// cronManager returns the tools.CronManager implementation backed by this App.
func (a *App) cronManager() tools.CronManager {
	return &appCronManager{app: a}
}

// emitCronsUpdated notifies the frontend that the cron task list has changed
// so the Claw Panel can refresh its display.
func (a *App) emitCronsUpdated() {
	if a.ctx == nil {
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "claw:crons:updated", nil)
}
