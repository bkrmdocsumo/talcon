package access

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/user/talon/internal/config"
)

// SenderInfo describes the sender of an incoming message.
type SenderInfo struct {
	ID          string // canonical sender key, e.g. "telegram:12345"
	DisplayName string
	Channel     string // "telegram", "http", "webhook"
	IsGroup     bool
	GroupID     string // non-empty for group messages
	MessageText string // needed for pairing code check
}

// Decision is the result of an access check.
type Decision struct {
	Allowed      bool
	Reason       string // "approved", "open_policy", "blocked", "pairing_required", "not_on_allowlist"
	PairingReply string // non-empty if a pairing prompt should be sent back
}

// Manager handles access control decisions and sender state persistence.
type Manager struct {
	mu      sync.RWMutex
	cfg     config.AccessControlConfig
	state   *config.AccessState
	baseDir string
}

// NewManager creates a Manager with the given config and loads persisted state.
func NewManager(baseDir string, cfg config.AccessControlConfig) *Manager {
	state, err := config.LoadAccessState(baseDir)
	if err != nil {
		log.Printf("[access] failed to load access state: %v", err)
		state = &config.AccessState{
			Senders:          make(map[string]config.SenderRecord),
			ChannelAgents:    make(map[string]string),
			ChannelAllowFrom: make(map[string][]string),
		}
	}
	return &Manager{
		cfg:     cfg,
		state:   state,
		baseDir: baseDir,
	}
}

// Config returns the current access control configuration.
func (m *Manager) Config() config.AccessControlConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// SetConfig updates the access control configuration.
func (m *Manager) SetConfig(cfg config.AccessControlConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// Check evaluates whether a sender is allowed to interact with the bot.
func (m *Manager) Check(sender SenderInfo) Decision {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Group messages use group policy.
	if sender.IsGroup {
		return m.checkGroup(sender)
	}

	// DM messages use DM policy.
	return m.checkDM(sender)
}

func (m *Manager) checkDM(sender SenderInfo) Decision {
	// Check if explicitly blocked.
	if rec, ok := m.state.Senders[sender.ID]; ok && rec.Status == "blocked" {
		return Decision{Allowed: false, Reason: "blocked"}
	}

	switch m.cfg.DMPolicy {
	case "open":
		// Track sender as approved if not already known.
		m.ensureSender(sender, "approved")
		return Decision{Allowed: true, Reason: "open_policy"}

	case "allowlist":
		if rec, ok := m.state.Senders[sender.ID]; ok && rec.Status == "approved" {
			return Decision{Allowed: true, Reason: "approved"}
		}
		// Also check channel-specific allowlists.
		if m.isOnChannelAllowlist(sender) {
			m.ensureSender(sender, "approved")
			return Decision{Allowed: true, Reason: "approved"}
		}
		m.ensureSender(sender, "pending")
		return Decision{Allowed: false, Reason: "not_on_allowlist"}

	case "pairing":
		// Already approved?
		if rec, ok := m.state.Senders[sender.ID]; ok && rec.Status == "approved" {
			return Decision{Allowed: true, Reason: "approved"}
		}
		// Check if the message contains the pairing code.
		if m.cfg.PairingCode != "" && strings.TrimSpace(sender.MessageText) == m.cfg.PairingCode {
			m.ensureSender(sender, "approved")
			m.save()
			return Decision{Allowed: true, Reason: "approved"}
		}
		// Send pairing prompt.
		m.ensureSender(sender, "pending")
		return Decision{
			Allowed:      false,
			Reason:       "pairing_required",
			PairingReply: "Please send the pairing code to start chatting.",
		}

	default:
		// Unknown policy — default to open.
		return Decision{Allowed: true, Reason: "open_policy"}
	}
}

func (m *Manager) checkGroup(sender SenderInfo) Decision {
	// Check if sender is explicitly blocked.
	if rec, ok := m.state.Senders[sender.ID]; ok && rec.Status == "blocked" {
		return Decision{Allowed: false, Reason: "blocked"}
	}

	switch m.cfg.GroupPolicy {
	case "open":
		return Decision{Allowed: true, Reason: "open_policy"}

	case "allowlist":
		if rec, ok := m.state.Senders[sender.ID]; ok && rec.Status == "approved" {
			return Decision{Allowed: true, Reason: "approved"}
		}
		if m.isOnChannelAllowlist(sender) {
			return Decision{Allowed: true, Reason: "approved"}
		}
		return Decision{Allowed: false, Reason: "not_on_allowlist"}

	default:
		return Decision{Allowed: true, Reason: "open_policy"}
	}
}

// MentionGating returns whether group messages require an @mention.
func (m *Manager) MentionGating() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.MentionGating
}

// ApproveSender marks a sender as approved.
func (m *Manager) ApproveSender(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rec, ok := m.state.Senders[id]; ok {
		rec.Status = "approved"
		rec.ApprovedAt = time.Now().Format(time.RFC3339)
		m.state.Senders[id] = rec
	} else {
		m.state.Senders[id] = config.SenderRecord{
			Status:     "approved",
			ApprovedAt: time.Now().Format(time.RFC3339),
		}
	}
	m.save()
}

// BlockSender marks a sender as blocked.
func (m *Manager) BlockSender(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rec, ok := m.state.Senders[id]; ok {
		rec.Status = "blocked"
		m.state.Senders[id] = rec
	} else {
		m.state.Senders[id] = config.SenderRecord{Status: "blocked"}
	}
	m.save()
}

// RemoveSender removes a sender from the state entirely.
func (m *Manager) RemoveSender(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.Senders, id)
	m.save()
}

// ListSenders returns all known sender records.
func (m *Manager) ListSenders() map[string]config.SenderRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]config.SenderRecord, len(m.state.Senders))
	for k, v := range m.state.Senders {
		out[k] = v
	}
	return out
}

// SetChannelAgent maps a channel ID to a specific agent name.
func (m *Manager) SetChannelAgent(channelID, agentName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if agentName == "" {
		delete(m.state.ChannelAgents, channelID)
	} else {
		m.state.ChannelAgents[channelID] = agentName
	}
	m.save()
}

// GetChannelAgent returns the agent override for a channel, or "" if none.
func (m *Manager) GetChannelAgent(channelID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.ChannelAgents[channelID]
}

// ChannelAgents returns a copy of all channel-agent mappings.
func (m *Manager) ChannelAgents() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.state.ChannelAgents))
	for k, v := range m.state.ChannelAgents {
		out[k] = v
	}
	return out
}

// ── internal helpers ──

func (m *Manager) ensureSender(sender SenderInfo, status string) {
	if _, ok := m.state.Senders[sender.ID]; ok {
		return
	}
	rec := config.SenderRecord{
		Status:      status,
		DisplayName: sender.DisplayName,
		Channel:     sender.Channel,
	}
	if status == "approved" {
		rec.ApprovedAt = time.Now().Format(time.RFC3339)
	}
	m.state.Senders[sender.ID] = rec
	m.save()
}

func (m *Manager) isOnChannelAllowlist(sender SenderInfo) bool {
	channelKey := sender.Channel
	if sender.IsGroup && sender.GroupID != "" {
		channelKey = sender.Channel + ":" + sender.GroupID
	}
	for _, allowed := range m.state.ChannelAllowFrom[channelKey] {
		if allowed == "*" || allowed == sender.ID {
			return true
		}
	}
	return false
}

func (m *Manager) save() {
	if err := config.SaveAccessState(m.baseDir, m.state); err != nil {
		log.Printf("[access] failed to save access state: %v", err)
	}
}
