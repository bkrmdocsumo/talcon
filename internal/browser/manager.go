package browser

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// Timeouts for browser operations.
const (
	navigateTimeout  = 30 * time.Second
	clickTimeout     = 15 * time.Second
	snapshotTimeout  = 10 * time.Second
	actionTimeout    = 10 * time.Second
	postClickWait    = 2 * time.Second
)

// Manager handles the browser lifecycle using chromedp.
// It automatically discovers Chrome/Chromium installed on the system
// (no separate browser install required).
type Manager struct {
	mu          sync.Mutex
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc

	// ElementIDs stores the set of interactive element IDs from the last snapshot.
	// Used by Act to validate target IDs before performing actions.
	ElementIDs map[int]bool
}

// NewManager creates a new browser manager (does not start the browser yet).
func NewManager() *Manager {
	return &Manager{
		ElementIDs: make(map[int]bool),
	}
}

// Start launches Chrome with the given headless setting.
// It uses the system-installed Chrome/Chromium — no separate install required.
func (m *Manager) Start(headless bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	m.allocCtx, m.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	m.ctx, m.cancel = chromedp.NewContext(m.allocCtx)

	// Run an empty action to ensure the browser actually starts.
	if err := chromedp.Run(m.ctx); err != nil {
		m.cancel()
		m.allocCancel()
		return fmt.Errorf("browser launch: %w", err)
	}

	return nil
}

// Close tears down the browser and allocator contexts.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
	}
	if m.allocCancel != nil {
		m.allocCancel()
	}
}

// Navigate goes to a URL, waits for the page to load, and returns a semantic snapshot.
func (m *Manager) Navigate(url string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	navCtx, navCancel := context.WithTimeout(m.ctx, navigateTimeout)
	defer navCancel()

	if err := chromedp.Run(navCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	); err != nil {
		return "", fmt.Errorf("navigate to %s: %w", url, err)
	}

	// Give JS-heavy pages a moment to render.
	_ = chromedp.Run(m.ctx, chromedp.Sleep(500*time.Millisecond))

	return m.takeSnapshotLocked()
}

// Act performs an action on the page and returns a fresh snapshot.
func (m *Manager) Act(action string, targetID int, text string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	selector := fmt.Sprintf(`[data-talon-id="%d"]`, targetID)

	switch action {
	case "click":
		if !m.ElementIDs[targetID] {
			return "", fmt.Errorf("element ID %d not found in current snapshot", targetID)
		}
		// Use a timeout so clicks that trigger navigation don't hang forever.
		clickCtx, clickCancel := context.WithTimeout(m.ctx, clickTimeout)
		defer clickCancel()

		// Use JavaScript click to avoid chromedp hanging on navigation-triggered clicks.
		// chromedp.Click can block when the page navigates away before the click
		// action promise resolves.
		jsClick := fmt.Sprintf(`document.querySelector('[data-talon-id="%d"]').click()`, targetID)
		if err := chromedp.Run(clickCtx, chromedp.Evaluate(jsClick, nil)); err != nil {
			return "", fmt.Errorf("click element %d: %w", targetID, err)
		}

		// Wait for the page to settle after the click (handles navigation).
		m.waitForPageReady()

	case "type":
		if !m.ElementIDs[targetID] {
			return "", fmt.Errorf("element ID %d not found in current snapshot", targetID)
		}
		actCtx, actCancel := context.WithTimeout(m.ctx, actionTimeout)
		defer actCancel()
		if err := chromedp.Run(actCtx,
			chromedp.Clear(selector, chromedp.ByQuery),
			chromedp.SendKeys(selector, text, chromedp.ByQuery),
		); err != nil {
			return "", fmt.Errorf("type into element %d: %w", targetID, err)
		}

	case "scroll":
		direction := 600
		if text == "up" {
			direction = -600
		}
		jsExpr := fmt.Sprintf("window.scrollBy(0, %d)", direction)
		actCtx, actCancel := context.WithTimeout(m.ctx, actionTimeout)
		defer actCancel()
		if err := chromedp.Run(actCtx, chromedp.Evaluate(jsExpr, nil)); err != nil {
			return "", fmt.Errorf("scroll: %w", err)
		}

	default:
		return "", fmt.Errorf("unknown action %q (expected click, type, or scroll)", action)
	}

	return m.takeSnapshotLocked()
}

// waitForPageReady waits for the page to be ready after a click that may
// have triggered navigation. Uses a timeout so it never blocks forever.
func (m *Manager) waitForPageReady() {
	waitCtx, waitCancel := context.WithTimeout(m.ctx, postClickWait)
	defer waitCancel()

	// First, sleep a moment to let any navigation start.
	_ = chromedp.Run(waitCtx, chromedp.Sleep(300*time.Millisecond))

	// Then wait for body to be ready (handles fresh navigations).
	if err := chromedp.Run(waitCtx, chromedp.WaitReady("body")); err != nil {
		log.Printf("[browser] page ready wait: %v (continuing with snapshot)", err)
	}

	// A bit more time for JS to render.
	_ = chromedp.Run(waitCtx, chromedp.Sleep(500*time.Millisecond))
}

// takeSnapshotLocked is the internal snapshot method (caller must hold m.mu).
func (m *Manager) takeSnapshotLocked() (string, error) {
	snapCtx, snapCancel := context.WithTimeout(m.ctx, snapshotTimeout)
	defer snapCancel()

	snapshot, elementIDs, err := TakeSnapshot(snapCtx)
	if err != nil {
		return "", fmt.Errorf("take snapshot: %w", err)
	}
	m.ElementIDs = elementIDs
	return snapshot, nil
}
