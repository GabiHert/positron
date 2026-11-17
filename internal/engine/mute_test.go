package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/claude"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/whatsapp"
)

func TestMuteChatGoesToWhatsAppThenTheStore(t *testing.T) {
	e, st, wa, _, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	const chat = "5511@s.whatsapp.net"
	if _, err := st.UpdateChat(chat, func(c *domain.Chat) { c.JID, c.Unread, c.NotifyOff = chat, 2, true }); err != nil {
		t.Fatal(err)
	}
	var badge []string
	e.d.OnUnread = func(c string) { badge = append(badge, c) }

	until := time.Now().Add(8 * time.Hour)
	c, err := e.MuteChat(context.Background(), chat, until)
	if err != nil || !c.Muted(time.Now()) || !c.MutedUntil.Equal(until) {
		t.Fatalf("mute 8h: %v %+v", err, c)
	}
	if len(wa.mutes) != 1 || !wa.mutes[0].mute || !wa.mutes[0].until.Equal(until) {
		t.Fatalf("WhatsApp got %+v", wa.mutes)
	}
	if len(badge) != 1 || st.TotalUnread(time.Now()) != 0 {
		t.Fatalf("badge %v total %d", badge, st.TotalUnread(time.Now()))
	}

	if c, _ = e.MuteChat(context.Background(), chat, domain.Forever); !c.Muted(time.Now().AddDate(50, 0, 0)) {
		t.Fatalf("always: %+v", c)
	}
	if !wa.mutes[1].until.Equal(whatsapp.MutedForever) {
		t.Fatalf("always went to WhatsApp as %v", wa.mutes[1].until)
	}

	// Unmuting clears WhatsApp's mute and the older Positron-only one.
	c, err = e.MuteChat(context.Background(), chat, time.Time{})
	if err != nil || c.Muted(time.Now()) || c.NotifyOff || wa.mutes[2].mute {
		t.Fatalf("unmute: %v %+v %+v", err, c, wa.mutes)
	}
	if st.TotalUnread(time.Now()) != 2 {
		t.Fatalf("badge after unmute %d", st.TotalUnread(time.Now()))
	}
}

func TestMuteChatKeepsTheStoreWhenWhatsAppRefuses(t *testing.T) {
	e, st, wa, _, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	const chat = "5511@s.whatsapp.net"
	_, _ = st.UpdateChat(chat, func(c *domain.Chat) { c.JID = chat })
	wa.muteErr = errors.New("offline")
	if _, err := e.MuteChat(context.Background(), chat, domain.Forever); err == nil {
		t.Fatal("expected the WhatsApp error")
	}
	if c, _ := st.Chat(chat); c.Muted(time.Now()) {
		t.Fatalf("stored a mute WhatsApp didn't take: %+v", c)
	}
	if _, err := e.MuteChat(context.Background(), "nobody@s.whatsapp.net", domain.Forever); err == nil {
		t.Fatal("an unknown chat should be refused")
	}
}

func TestMuteFromThePhoneIsStored(t *testing.T) {
	e, st, _, _, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	const chat = "5511@s.whatsapp.net"
	changed := 0
	e.d.OnChats = func() { changed++ }
	now := time.Now()

	e.HandleUpdate(whatsapp.ChatMeta{Chat: chat, Field: "mute", On: true}) // no end: always
	if c, _ := st.Chat(chat); !c.MutedUntil.Equal(domain.Forever) {
		t.Fatalf("always: %v", c.MutedUntil)
	}
	week := now.Add(7 * 24 * time.Hour)
	e.HandleUpdate(whatsapp.ChatMeta{Chat: chat, Field: "mute", On: true, Until: week})
	if c, _ := st.Chat(chat); !c.MutedUntil.Equal(week) || !c.Muted(now) || c.Muted(week.Add(time.Second)) {
		t.Fatalf("a week: %v", c.MutedUntil)
	}
	e.HandleUpdate(whatsapp.ChatMeta{Chat: chat, Field: "mute", On: false, Until: week})
	if c, _ := st.Chat(chat); c.Muted(now) || !c.MutedUntil.IsZero() {
		t.Fatalf("unmuted: %v", c.MutedUntil)
	}
	if changed != 3 {
		t.Fatalf("chat list changed %d times", changed)
	}

	// A history sync carries the phone's mute too, "always" included.
	e.HandleHistory(whatsapp.History{Type: "recent", Chats: []whatsapp.HistoryChat{{JID: chat, HasMeta: true, MutedUntil: whatsapp.MutedForever}}})
	if c, _ := st.Chat(chat); !c.Muted(now.AddDate(100, 0, 0)) {
		t.Fatalf("history always: %v", c.MutedUntil)
	}
}

// Mute is about notifications: the chat's agent still answers.
func TestAgentsStillAnswerMutedChats(t *testing.T) {
	e, st, wa, rn, _ := setup(t, domain.ModeAuto, domain.TriggerMention)
	_, _ = st.UpdateChat("group1@g.us", func(c *domain.Chat) { c.JID, c.IsGroup, c.MutedUntil = "group1@g.us", true, domain.Forever })
	rn.res = claude.AgentResult{Reply: "Oi!"}
	e.HandleIncoming(msg("group1@g.us", "M1", "@positron oi", true, true))
	if r := waitRun(t, e); r.Status != domain.RunSent || len(wa.all()) != 1 {
		t.Fatalf("run %s, sent %+v", r.Status, wa.all())
	}
}
