package claw

import (
	"time"

	"github.com/google/uuid"
)

// EventType identifies the kind of trigger that produced an AgentEvent.
type EventType string

const (
	EventMessage   EventType = "message"
	EventHeartbeat EventType = "heartbeat"
	EventCron      EventType = "cron"
	EventHook      EventType = "hook"
	EventWebhook   EventType = "webhook"
)

// EventStatus tracks the lifecycle of an event in the gateway queue.
type EventStatus string

const (
	StatusPending    EventStatus = "pending"
	StatusProcessing EventStatus = "processing"
	StatusCompleted  EventStatus = "completed"
	StatusSuppressed EventStatus = "suppressed" // heartbeat OK, no user-visible output
	StatusFailed     EventStatus = "failed"
)

// AgentEvent is the standardized unit that flows through the Gateway queue.
// Every input source (chat, cron, webhook, heartbeat, hook) produces one of these.
type AgentEvent struct {
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	Payload   string            `json:"payload"`
	Source    string            `json:"source"`
	SessionID string            `json:"session_id"`
	AgentName string            `json:"agent_name"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Hidden    bool              `json:"hidden"`
}

// EventRecord pairs an event with its processing outcome so the UI can
// display a full history of what the gateway has handled.
type EventRecord struct {
	Event       AgentEvent  `json:"event"`
	Status      EventStatus `json:"status"`
	Result      string      `json:"result"`
	ProcessedAt time.Time   `json:"processed_at"`
}

// NewEvent creates an AgentEvent with a fresh UUID and timestamp.
func NewEvent(typ EventType, payload, source, sessionID, agentName string) AgentEvent {
	return AgentEvent{
		ID:        uuid.New().String(),
		Type:      typ,
		Payload:   payload,
		Source:    source,
		SessionID: sessionID,
		AgentName: agentName,
		Timestamp: time.Now(),
	}
}
