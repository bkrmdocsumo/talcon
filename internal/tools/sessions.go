package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SessionLister provides session listing and history for the session tools.
type SessionLister interface {
	ListSessionsByPrefix(prefix string) ([]SessionListEntry, error)
	LoadSessionHistory(sessionID string, lastN int) ([]SessionMessage, error)
}

// SessionListEntry is a simplified session descriptor returned by list.
type SessionListEntry struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
}

// SessionMessage is a simplified message returned by history.
type SessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SessionEventPusher pushes a message into another session via the gateway.
type SessionEventPusher interface {
	PushSessionMessage(sessionID, message string)
}

type sessionListerKey struct{}
type sessionPusherKey struct{}

// WithSessionLister returns a context carrying a SessionLister.
func WithSessionLister(ctx context.Context, lister SessionLister) context.Context {
	return context.WithValue(ctx, sessionListerKey{}, lister)
}

// SessionListerFromContext extracts the SessionLister from the context.
func SessionListerFromContext(ctx context.Context) SessionLister {
	l, _ := ctx.Value(sessionListerKey{}).(SessionLister)
	return l
}

// WithSessionPusher returns a context carrying a SessionEventPusher.
func WithSessionPusher(ctx context.Context, pusher SessionEventPusher) context.Context {
	return context.WithValue(ctx, sessionPusherKey{}, pusher)
}

// SessionPusherFromContext extracts the SessionEventPusher from the context.
func SessionPusherFromContext(ctx context.Context) SessionEventPusher {
	p, _ := ctx.Value(sessionPusherKey{}).(SessionEventPusher)
	return p
}

// ─── sessions_list tool ───

// SessionsListTool lists active sessions, optionally filtered by prefix.
type SessionsListTool struct{}

func (t *SessionsListTool) Name() string { return "sessions_list" }

func (t *SessionsListTool) Description() string {
	return `List active sessions. Returns session IDs and last-modified timestamps.
Use the optional "prefix" parameter to filter sessions (e.g. "agent_main" to see main agent sessions, "tg_" for Telegram sessions).`
}

func (t *SessionsListTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prefix": map[string]interface{}{
				"type":        "string",
				"description": "Optional prefix to filter session IDs.",
			},
		},
	}
}

func (t *SessionsListTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	lister := SessionListerFromContext(ctx)
	if lister == nil {
		return "Error: session listing is not available in this mode", nil
	}

	var in struct {
		Prefix string `json:"prefix"`
	}
	json.Unmarshal(input, &in)

	entries, err := lister.ListSessionsByPrefix(in.Prefix)
	if err != nil {
		return fmt.Sprintf("Error listing sessions: %v", err), nil
	}

	if len(entries) == 0 {
		return "No sessions found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Sessions (%d):\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(&b, "  - %s (modified: %d)\n", e.ID, e.Timestamp)
	}
	return b.String(), nil
}

// ─── sessions_history tool ───

// SessionsHistoryTool reads recent messages from a session.
type SessionsHistoryTool struct{}

func (t *SessionsHistoryTool) Name() string { return "sessions_history" }

func (t *SessionsHistoryTool) Description() string {
	return `Read recent messages from another session's conversation history.
Use this to see what another agent or peer has been discussing.`
}

func (t *SessionsHistoryTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "The session ID to read history from.",
			},
			"last_n": map[string]interface{}{
				"type":        "integer",
				"description": "Number of recent messages to return (default: 10).",
			},
		},
		"required": []string{"session_id"},
	}
}

func (t *SessionsHistoryTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	lister := SessionListerFromContext(ctx)
	if lister == nil {
		return "Error: session history is not available in this mode", nil
	}

	var in struct {
		SessionID string `json:"session_id"`
		LastN     int    `json:"last_n"`
	}
	json.Unmarshal(input, &in)

	if in.SessionID == "" {
		return "Error: session_id is required", nil
	}
	if in.LastN <= 0 {
		in.LastN = 10
	}

	msgs, err := lister.LoadSessionHistory(in.SessionID, in.LastN)
	if err != nil {
		return fmt.Sprintf("Error loading session history: %v", err), nil
	}

	if len(msgs) == 0 {
		return fmt.Sprintf("Session %q has no messages.", in.SessionID), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Last %d messages from session %q:\n\n", len(msgs), in.SessionID)
	for _, m := range msgs {
		content := m.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		fmt.Fprintf(&b, "[%s]: %s\n\n", m.Role, content)
	}
	return b.String(), nil
}

// ─── sessions_send tool ───

// SessionsSendTool sends a message into another session.
type SessionsSendTool struct{}

func (t *SessionsSendTool) Name() string { return "sessions_send" }

func (t *SessionsSendTool) Description() string {
	return `Send a message to another agent's session. The message will be processed as if a user sent it to that session.
Use this for inter-agent communication — for example, to delegate a task to a specialized agent or to share information across sessions.`
}

func (t *SessionsSendTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "The target session ID to send the message to.",
			},
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The message to send.",
			},
		},
		"required": []string{"session_id", "message"},
	}
}

func (t *SessionsSendTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	pusher := SessionPusherFromContext(ctx)
	if pusher == nil {
		return "Error: session messaging is not available in this mode", nil
	}

	var in struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	json.Unmarshal(input, &in)

	if in.SessionID == "" {
		return "Error: session_id is required", nil
	}
	if in.Message == "" {
		return "Error: message is required", nil
	}

	pusher.PushSessionMessage(in.SessionID, in.Message)
	return fmt.Sprintf("Message sent to session %q.", in.SessionID), nil
}
