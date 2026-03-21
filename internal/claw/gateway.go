package claw

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/user/talon/internal/agent"
	"github.com/user/talon/internal/config"
	"github.com/user/talon/internal/session"
	"github.com/user/talon/internal/tools"
)

const (
	defaultQueueSize  = 100
	maxHistoryRecords = 500
)

// EmitFunc is the callback signature used to push events to the frontend.
type EmitFunc func(eventName string, data interface{})

// Gateway is the centralized event queue that serialises all inputs
// (messages, heartbeats, crons, hooks, webhooks) before feeding them
// to the agent one at a time.
type Gateway struct {
	queue   chan AgentEvent
	ctx     context.Context
	cancel  context.CancelFunc
	cfg     *config.Config
	deps    agent.Deps
	emit    EmitFunc
	wg      sync.WaitGroup
	cronMgr tools.CronManager

	histMu  sync.RWMutex
	history []EventRecord
}

// NewGateway creates a gateway with a buffered event channel.
func NewGateway(ctx context.Context, cfg *config.Config, deps agent.Deps, emit EmitFunc) *Gateway {
	gCtx, cancel := context.WithCancel(ctx)
	return &Gateway{
		queue:   make(chan AgentEvent, defaultQueueSize),
		ctx:     gCtx,
		cancel:  cancel,
		cfg:     cfg,
		deps:    deps,
		emit:    emit,
		history: make([]EventRecord, 0, maxHistoryRecords),
	}
}

// Push enqueues an event. Non-blocking: drops the event if the queue is full.
func (g *Gateway) Push(evt AgentEvent) {
	// Emit/store the event as pending immediately so queued items are visible
	// in the UI even before processing starts.
	pending := EventRecord{
		Event:       evt,
		Status:      StatusPending,
		ProcessedAt: time.Now(),
	}
	g.upsertRecord(pending)
	g.emitClawEvent(evt, StatusPending, "")

	select {
	case g.queue <- evt:
		log.Printf("[claw] queued event %s type=%s source=%s", evt.ID, evt.Type, evt.Source)
	default:
		log.Printf("[claw] WARNING: queue full, dropping event %s", evt.ID)
		failed := EventRecord{
			Event:       evt,
			Status:      StatusFailed,
			Result:      "error: queue full, event dropped",
			ProcessedAt: time.Now(),
		}
		g.upsertRecord(failed)
		g.emitClawEvent(evt, StatusFailed, failed.Result)
	}
}

// Start launches the background goroutine that consumes events sequentially.
func (g *Gateway) Start() {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		log.Println("[claw] gateway loop started")
		for {
			select {
			case <-g.ctx.Done():
				log.Println("[claw] gateway loop stopped")
				return
			case evt := <-g.queue:
				g.processEvent(evt)
			}
		}
	}()
}

// Stop cancels the gateway loop and waits for it to finish.
func (g *Gateway) Stop() {
	g.cancel()
	g.wg.Wait()
}

// SetCronManager sets the CronManager used to inject cron management
// capabilities into agent turns processed by this gateway.
func (g *Gateway) SetCronManager(mgr tools.CronManager) {
	g.cronMgr = mgr
}

// History returns a copy of the event history (newest first).
func (g *Gateway) History() []EventRecord {
	g.histMu.RLock()
	defer g.histMu.RUnlock()
	out := make([]EventRecord, len(g.history))
	copy(out, g.history)
	// Reverse so newest is first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Stats returns aggregate statistics about processed events.
func (g *Gateway) Stats() map[string]interface{} {
	g.histMu.RLock()
	defer g.histMu.RUnlock()

	today := time.Now().Truncate(24 * time.Hour)
	totalToday := 0
	var lastHeartbeat time.Time
	typeCounts := map[EventType]int{}

	for _, rec := range g.history {
		typeCounts[rec.Event.Type]++
		if rec.Event.Timestamp.After(today) {
			totalToday++
		}
		if rec.Event.Type == EventHeartbeat && rec.ProcessedAt.After(lastHeartbeat) {
			lastHeartbeat = rec.ProcessedAt
		}
	}

	return map[string]interface{}{
		"total":           len(g.history),
		"today":           totalToday,
		"last_heartbeat":  lastHeartbeat,
		"counts_by_type":  typeCounts,
	}
}

func (g *Gateway) processEvent(evt AgentEvent) {
	log.Printf("[claw] processing event %s type=%s", evt.ID, evt.Type)

	// Notify frontend that event processing has started.
	g.emitClawEvent(evt, StatusProcessing, "")

	agentCfg, ok := g.cfg.Agents[evt.AgentName]
	if !ok {
		agentCfg = g.cfg.Agents["main"]
	}

	content, _ := json.Marshal(evt.Payload)

	deps := g.deps
	deps.ChatMode = true

	ctx := g.ctx
	if g.cronMgr != nil {
		ctx = tools.WithCronManager(ctx, g.cronMgr)
	}

	// For cron events, restore the workspace directory from the session
	// that created the cron so the agent can find files from that session.
	if evt.Type == EventCron {
		if wsDir := evt.Metadata["workspace_dir"]; wsDir != "" {
			ctx = tools.WithSessionDir(ctx, wsDir)
		}
	}

	// Resolve session ID based on agent's session scope when sender/channel
	// info is available on the event.
	resolvedSession := evt.SessionID
	if evt.SenderID != "" && agentCfg.SessionScope != "" {
		resolvedSession = session.ResolveSessionID(
			agentCfg.SessionScope,
			agentCfg.SessionPrefix,
			evt.SenderID,
			evt.ChannelID,
		)
	}

	result, err := agent.RunAgentTurn(ctx, resolvedSession, content, agentCfg, deps)

	rec := EventRecord{
		Event:       evt,
		ProcessedAt: time.Now(),
	}

	if err != nil {
		log.Printf("[claw] event %s failed: %v", evt.ID, err)
		rec.Status = StatusFailed
		rec.Result = fmt.Sprintf("error: %v", err)
	} else {
		rec.Result = result.FinalText

		// Heartbeat suppression: if the agent says everything is fine, hide it.
		if evt.Type == EventHeartbeat && strings.Contains(result.FinalText, "[HEARTBEAT_OK]") {
			rec.Status = StatusSuppressed
			log.Printf("[claw] heartbeat %s suppressed (OK)", evt.ID)
		} else {
			rec.Status = StatusCompleted
		}
	}

	g.upsertRecord(rec)
	g.emitClawEvent(evt, rec.Status, rec.Result)
}

func (g *Gateway) upsertRecord(rec EventRecord) {
	g.histMu.Lock()
	defer g.histMu.Unlock()
	for i := range g.history {
		if g.history[i].Event.ID == rec.Event.ID {
			g.history[i] = rec
			return
		}
	}
	g.history = append(g.history, rec)
	if len(g.history) > maxHistoryRecords {
		g.history = g.history[len(g.history)-maxHistoryRecords:]
	}
}

func (g *Gateway) emitClawEvent(evt AgentEvent, status EventStatus, result string) {
	if g.emit == nil {
		return
	}
	g.emit("claw:event", map[string]interface{}{
		"id":         evt.ID,
		"type":       string(evt.Type),
		"source":     evt.Source,
		"payload":    evt.Payload,
		"session_id": evt.SessionID,
		"status":     string(status),
		"result":     result,
		"timestamp":  evt.Timestamp.Format(time.RFC3339),
		"hidden":     evt.Hidden,
	})
}
