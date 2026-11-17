package backend

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/store"
)

func TestShouldNotifyMessage(t *testing.T) {
	now := time.Now()
	live := domain.Message{ChatJID: "c", At: now.Add(-time.Minute), Kind: domain.KindText, Text: "oi"}
	if !shouldNotifyMessage(live, now) {
		t.Fatal("a live message from someone else should notify")
	}
	for name, m := range map[string]domain.Message{
		"mine":    {FromMe: true, At: now},
		"agent":   {FromAgent: true, At: now},
		"history": {History: true, At: now},
		"backlog": {Offline: true, At: now},
		"deleted": {Revoked: true, At: now},
		"too old": {At: now.Add(-2 * time.Hour)},
	} {
		if shouldNotifyMessage(m, now) {
			t.Errorf("%s: should not notify", name)
		}
	}
}

func TestChatAllows(t *testing.T) {
	now := time.Now()
	s := domain.DefaultSettings()
	direct := domain.Chat{JID: "a", Name: "Ana", Unread: 1}
	group := domain.Chat{JID: "g", Name: "Família", IsGroup: true, Unread: 1}
	plain := domain.Message{Text: "oi"}
	mention := domain.Message{Text: "@you", MentionsMe: true}
	off := s
	off.NotifyMessages = false
	mentions := s
	mentions.NotifyGroups = domain.GroupsMentions
	noGroups := s
	noGroups.NotifyGroups = domain.GroupsOff
	muted := direct
	muted.MutedUntil = now.Add(time.Hour)
	mutedPast := direct
	mutedPast.MutedUntil = now.Add(-time.Hour)
	silenced := direct
	silenced.NotifyOff = true
	archived := direct
	archived.Archived = true
	read := direct
	read.Unread = 0
	for _, c := range []struct {
		name string
		s    domain.Settings
		c    domain.Chat
		m    domain.Message
		open bool
		want bool
	}{
		{"direct", s, direct, plain, false, true},
		{"switched off", off, direct, plain, false, false},
		{"open and focused", s, direct, plain, true, false},
		{"muted on WhatsApp", s, muted, plain, false, false},
		{"mute ended", s, mutedPast, plain, false, true},
		{"muted in Positron", s, silenced, plain, false, false},
		{"archived", s, archived, plain, false, false},
		{"read meanwhile", s, read, plain, false, false},
		{"group", s, group, plain, false, true},
		{"group, mentions only", mentions, group, plain, false, false},
		{"group, mentioned", mentions, group, mention, false, true},
		{"direct ignores the group rule", mentions, direct, plain, false, true},
		{"groups off", noGroups, group, mention, false, false},
	} {
		if got := chatAllows(c.s, c.c, c.m, c.open, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNotificationText(t *testing.T) {
	s := domain.DefaultSettings()
	hidden := s
	hidden.NotifyPreview = false
	ana := domain.Chat{Name: "Ana", Unread: 1}
	fam := domain.Chat{Name: "Família", IsGroup: true, Unread: 3}
	photo := domain.Message{Kind: domain.KindImage, Text: "a praia", SenderName: "Ana"}
	voice := domain.Message{Kind: domain.KindVoice, Seconds: 9, SenderName: "Tiago", SenderJID: "5551@s.whatsapp.net"}
	for _, c := range []struct {
		name                  string
		s                     domain.Settings
		c                     domain.Chat
		m                     domain.Message
		title, subtitle, body string
	}{
		{"direct photo", s, ana, photo, "Ana", "", "📷 Photo a praia"},
		{"group, bundled", s, fam, voice, "Tiago @ Família", "3 new messages", "🎤 Voice note (0:09)"},
		{"group, unknown sender", s, fam, domain.Message{Kind: domain.KindText, Text: "oi", SenderJID: "5551@s.whatsapp.net"}, "+5551 @ Família", "3 new messages", "oi"},
		{"no preview", hidden, ana, photo, "Ana", "", "New message"},
		{"no preview, bundled group", hidden, fam, voice, "Família", "", "3 new messages"},
	} {
		title, sub, body := notificationText(c.s, c.c, c.m)
		if title != c.title || sub != c.subtitle || body != c.body {
			t.Errorf("%s: got %q / %q / %q", c.name, title, sub, body)
		}
	}
}

func TestBurstsBecomeOneNotification(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_ = st.SaveChat(domain.Chat{JID: "a", Name: "Ana", Unread: 3})
	_ = st.SaveChat(domain.Chat{JID: "b", Name: "Bia", Unread: 1})
	var mu sync.Mutex
	var got []ChatNotification
	var cleared []string
	b := &Backend{d: Deps{Store: st,
		NotifyChat:        func(n ChatNotification) { mu.Lock(); got = append(got, n); mu.Unlock() },
		ClearNotification: func(id string) { mu.Lock(); cleared = append(cleared, id); mu.Unlock() },
	}}
	b.nt().gather = 50 * time.Millisecond
	now := time.Now()
	for i, text := range []string{"oi", "tudo bem?", "me liga"} {
		b.queueNotify(domain.Message{ID: string(rune('1' + i)), ChatJID: "a", At: now, Kind: domain.KindText, Text: text})
	}
	b.queueNotify(domain.Message{ID: "x", ChatJID: "b", At: now, Kind: domain.KindText, Text: "oi"})
	b.queueNotify(domain.Message{ID: "mine", ChatJID: "b", FromMe: true, At: now, Kind: domain.KindText, Text: "eu"})
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	if len(got) != 2 {
		t.Fatalf("got %d notifications: %+v", len(got), got)
	}
	byChat := map[string]ChatNotification{got[0].Chat: got[0], got[1].Chat: got[1]}
	if a := byChat["a"]; a.Subtitle != "3 new messages" || a.Body != "me liga" || !a.Sound {
		t.Errorf("a: %+v", a)
	}
	if bb := byChat["b"]; bb.Body != "oi" || bb.Subtitle != "" {
		t.Errorf("b: %+v", bb)
	}
	got = nil
	mu.Unlock()

	// Read before the notification went out: nothing shows, and the chat's banner is taken away.
	b.queueNotify(domain.Message{ID: "4", ChatJID: "a", At: now, Kind: domain.KindText, Text: "?"})
	_, _ = st.UpdateChat("a", func(c *domain.Chat) { c.Unread = 0 })
	b.OnUnread("a")
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 0 || len(cleared) == 0 || cleared[len(cleared)-1] != "chat:a" {
		t.Fatalf("got %+v, cleared %v", got, cleared)
	}
}

func TestMuteInPositron(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_ = st.SaveChat(domain.Chat{JID: "a", Name: "Ana", Unread: 2})
	_ = st.SaveChat(domain.Chat{JID: "b", Name: "Bia", Unread: 5})
	var badge []int
	b := &Backend{d: Deps{Store: st, OnUnreadTotal: func(n int) { badge = append(badge, n) }}}
	if st.TotalUnread(time.Now()) != 7 {
		t.Fatal("badge before")
	}
	if err := b.SetChatNotifications("b", false); err != nil {
		t.Fatal(err)
	}
	if c, _ := st.Chat("b"); !c.NotifyOff || badge[len(badge)-1] != 2 {
		t.Fatalf("after mute: %+v badge %v", c, badge)
	}
	_ = b.SetChatNotifications("b", true)
	if badge[len(badge)-1] != 7 {
		t.Fatalf("after unmute: badge %v", badge)
	}
	if b.GetSettings().NotifyGroups != domain.GroupsAll {
		t.Fatal("default groups setting")
	}
}
