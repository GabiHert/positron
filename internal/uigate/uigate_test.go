package uigate

import (
	"reflect"
	"sync"
	"testing"
)

type recorder struct {
	mu  sync.Mutex
	got []string
}

func (r *recorder) emit(event string, _ any) {
	r.mu.Lock()
	r.got = append(r.got, event)
	r.mu.Unlock()
}

func TestHiddenWindowGetsNoEventsThenOneResume(t *testing.T) {
	r := &recorder{}
	g := New(r.emit, "navigate")
	g.Emit("activity", 1)
	g.SetVisible(false)
	g.Emit("activity", 2)
	g.Emit("wa:message", 3)
	g.Emit("navigate", "inbox") // the window needs it when it shows
	if g.Skipped() != 2 || !g.Hidden() {
		t.Fatalf("skipped %d hidden %v", g.Skipped(), g.Hidden())
	}
	g.SetVisible(true)
	g.SetVisible(true) // showing again sends nothing more
	g.Emit("chats", nil)
	want := []string{"activity", "navigate", EventResume, "chats"}
	if !reflect.DeepEqual(r.got, want) {
		t.Fatalf("events %v, want %v", r.got, want)
	}
}

func TestShowingWithoutSkippedEventsSendsNoResume(t *testing.T) {
	r := &recorder{}
	g := New(r.emit)
	g.SetVisible(false)
	g.SetVisible(true)
	if len(r.got) != 0 {
		t.Fatalf("events %v, want none", r.got)
	}
}
