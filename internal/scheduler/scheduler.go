package scheduler

import (
	"context"
	"encoding/json"
	"log"

	"github.com/robfig/cron/v3"
	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/config"
)

// Task represents a scheduled task that triggers an agent turn.
type Task struct {
	Schedule  string // cron expression, e.g. "0 8 * * *"
	SessionID string // unique session, e.g. "cron:morning"
	AgentName string // which agent config to use
	Message   string // the prompt to send
}

// Scheduler wraps robfig/cron to run periodic agent tasks.
type Scheduler struct {
	cron *cron.Cron
	ctx  context.Context
}

// New creates a new scheduler.
func New(ctx context.Context) *Scheduler {
	return &Scheduler{
		cron: cron.New(),
		ctx:  ctx,
	}
}

// AddTask registers a scheduled task.
func (s *Scheduler) AddTask(task Task, cfg *config.Config, deps agent.Deps) error {
	agentCfg, ok := cfg.Agents[task.AgentName]
	if !ok {
		agentCfg = cfg.Agents["main"]
	}

	_, err := s.cron.AddFunc(task.Schedule, func() {
		log.Printf("[cron] running task %q (session=%s)", task.Message, task.SessionID)

		content, _ := json.Marshal(task.Message)
		result, err := agent.RunAgentTurn(s.ctx, task.SessionID, content, agentCfg, deps)
		if err != nil {
			log.Printf("[cron] error: %v", err)
			return
		}

		log.Printf("[cron] response (%d chars): %s", len(result.FinalText), truncate(result.FinalText, 200))
	})

	return err
}

// Start begins the cron scheduler.
func (s *Scheduler) Start() {
	log.Println("[cron] scheduler started")
	s.cron.Start()
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	log.Println("[cron] scheduler stopping")
	s.cron.Stop()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
