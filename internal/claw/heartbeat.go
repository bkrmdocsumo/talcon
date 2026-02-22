package claw

import (
	"log"
	"sync"
	"time"
)

const (
	DefaultHeartbeatInterval = 1 * time.Minute
	heartbeatPrompt          = `Check your tools, emails, and notifications for anything urgent or noteworthy. If nothing requires attention, reply with exactly [HEARTBEAT_OK] and nothing else. If you find something important, summarise it for the user.`
	heartbeatSessionID       = "claw:heartbeat"
	heartbeatAgentName       = "main"
)

// Heartbeat runs a periodic ticker that pushes heartbeat events into
// the Gateway queue so the agent can proactively check for updates.
type Heartbeat struct {
	gateway  *Gateway
	interval time.Duration
	ticker   *time.Ticker
	stopCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewHeartbeat creates a heartbeat with the given interval.
// Pass 0 to use the default 30-minute interval.
func NewHeartbeat(gw *Gateway, interval time.Duration) *Heartbeat {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	return &Heartbeat{
		gateway:  gw,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the heartbeat ticker in the background.
func (h *Heartbeat) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		return
	}
	h.running = true
	h.stopCh = make(chan struct{})
	h.ticker = time.NewTicker(h.interval)
	go h.loop()
	log.Printf("[claw] heartbeat started (interval=%s)", h.interval)
}

// Stop halts the heartbeat ticker.
func (h *Heartbeat) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return
	}
	h.running = false
	h.ticker.Stop()
	close(h.stopCh)
	log.Println("[claw] heartbeat stopped")
}

// IsRunning returns whether the heartbeat ticker is active.
func (h *Heartbeat) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}

// Interval returns the current heartbeat interval.
func (h *Heartbeat) Interval() time.Duration {
	return h.interval
}

// SetInterval updates the interval. Requires a Stop()/Start() cycle to take effect.
func (h *Heartbeat) SetInterval(d time.Duration) {
	if d > 0 {
		h.interval = d
	}
}

// FireNow pushes a heartbeat event immediately (for manual testing).
func (h *Heartbeat) FireNow() {
	evt := NewEvent(EventHeartbeat, heartbeatPrompt, "heartbeat-ticker", heartbeatSessionID, heartbeatAgentName)
	evt.Hidden = true
	h.gateway.Push(evt)
}

func (h *Heartbeat) loop() {
	for {
		select {
		case <-h.stopCh:
			return
		case <-h.ticker.C:
			h.FireNow()
		}
	}
}
