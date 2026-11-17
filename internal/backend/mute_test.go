package backend

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/engine"
	"github.com/GabiHert/positron/internal/store"
)

// muteWA is a WhatsApp link that only mutes.
type muteWA struct {
	engine.WhatsApp
	mu    sync.Mutex
	calls []bool
	err   error
}

func (w *muteWA) SetMute(_ context.Context, _ string, mute bool, _ time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	w.calls = append(w.calls, mute)
	return nil
}

func TestMuteEnd(t *testing.T) {
	now := time.Date(2026, 11, 17, 21, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"8h":      now.Add(8 * time.Hour),
		"1w":      now.AddDate(0, 0, 7),
		"1 week":  now.AddDate(0, 0, 7),
		"Always":  domain.Forever,
		"off":     {},
		" unmute": {},
	} {
		if got, err := muteEnd(in, now); err != nil || !got.Equal(want) {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := muteEnd("3 days", now); err == nil {
		t.Error("3 days isn't one of WhatsApp's choices")
	}
}

func TestMuteChat(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_ = st.SaveChat(domain.Chat{JID: "a", Name: "Ana", Unread: 2})
	_ = st.SaveChat(domain.Chat{JID: "b", Name: "Bia", Unread: 5})
	wa := &muteWA{}
	var mu sync.Mutex
	var badge []int
	var got []ChatNotification
	b := &Backend{d: Deps{Store: st,
		OnUnreadTotal: func(n int) { mu.Lock(); badge = append(badge, n); mu.Unlock() },
		NotifyChat:    func(n ChatNotification) { mu.Lock(); got = append(got, n); mu.Unlock() },
	}}
	b.d.Engine = engine.New(engine.Deps{Store: st, WA: wa, OnUnread: b.OnUnread})
	t.Cleanup(b.d.Engine.Close)
	b.nt().gather = 50 * time.Millisecond

	// A message is waiting to notify when the chat gets muted: it never shows.
	b.queueNotify(domain.Message{ID: "1", ChatJID: "b", At: time.Now(), Kind: domain.KindText, Text: "oi"})
	until, err := b.MuteChat("b", "8h")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(time.UnixMilli(until)); d < 7*time.Hour || d > 8*time.Hour {
		t.Fatalf("muted until %v", time.UnixMilli(until))
	}
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	if len(got) != 0 || len(badge) == 0 || badge[len(badge)-1] != 2 {
		t.Fatalf("notified %+v, badge %v", got, badge)
	}
	mu.Unlock()
	if cs := b.ListChats(); muted(cs, "b") != until || muted(cs, "a") != 0 {
		t.Fatalf("ListChats mutes: %+v", cs)
	}

	// Muted chats still count as unread, they just leave the badge.
	if c, _ := st.Chat("b"); c.Unread != 5 {
		t.Fatalf("unread %d", c.Unread)
	}

	if until, _ = b.MuteChat("b", "always"); until != domain.Forever.UnixMilli() {
		t.Fatalf("always = %d", until)
	}
	if until, err = b.MuteChat("b", "off"); err != nil || until != 0 {
		t.Fatalf("off: %d %v", until, err)
	}
	mu.Lock()
	if badge[len(badge)-1] != 7 {
		t.Fatalf("badge after unmute %v", badge)
	}
	mu.Unlock()
	if len(wa.calls) != 3 || !wa.calls[0] || !wa.calls[1] || wa.calls[2] {
		t.Fatalf("WhatsApp got %v", wa.calls)
	}

	wa.err = errors.New("not connected")
	if _, err := b.MuteChat("a", "8h"); err == nil {
		t.Fatal("expected WhatsApp's error")
	}
	if _, err := b.MuteChat("a", "a fortnight"); err == nil {
		t.Fatal("expected a length error")
	}
}

func muted(cs []app.ChatDTO, jid string) int64 {
	for _, c := range cs {
		if c.JID == jid {
			return c.MutedUntil
		}
	}
	return -1
}
