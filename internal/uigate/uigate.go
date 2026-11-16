// Package uigate stops UI events while Positron's window is hidden. WhatsApp, the agents and the
// MCP server keep running; only the events that redraw the window wait. When the window comes
// back after events were held, the gate sends one resume event so the window reloads its data.
package uigate

import "sync"

// EventResume tells the window that events were skipped while it was hidden.
const EventResume = "resume"

// Gate passes events to emit while the window is visible.
type Gate struct {
	emit func(event string, data any)

	mu      sync.Mutex
	hidden  bool
	skipped int
	always  map[string]bool
}

// New returns a visible gate. Events named in always pass even while hidden, for events the
// window needs the moment it shows (such as which view to open).
func New(emit func(event string, data any), always ...string) *Gate {
	g := &Gate{emit: emit, always: map[string]bool{}}
	for _, e := range always {
		g.always[e] = true
	}
	return g
}

// Emit sends event, or skips it while the window is hidden.
func (g *Gate) Emit(event string, data any) {
	g.mu.Lock()
	if g.hidden && !g.always[event] {
		g.skipped++
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()
	g.emit(event, data)
}

// SetVisible records whether the window can be seen. Showing it after skipped events sends
// EventResume once.
func (g *Gate) SetVisible(visible bool) {
	g.mu.Lock()
	resume := visible && g.hidden && g.skipped > 0
	g.hidden = !visible
	if visible {
		g.skipped = 0
	}
	g.mu.Unlock()
	if resume {
		g.emit(EventResume, nil)
	}
}

// Hidden reports whether the window is hidden.
func (g *Gate) Hidden() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hidden
}

// Skipped is how many events were held since the window was hidden.
func (g *Gate) Skipped() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.skipped
}
