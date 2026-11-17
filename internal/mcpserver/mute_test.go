package mcpserver

import (
	"strings"
	"testing"
	"time"
)

func (f *fakeBackend) MuteChat(jid, length string) (int64, error) {
	var until int64
	switch length {
	case "always":
		until = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	case "8h":
		until = time.Date(2026, 11, 18, 5, 0, 0, 0, time.Local).UnixMilli()
	}
	for i := range f.chats {
		if f.chats[i].JID == jid {
			f.chats[i].MutedUntil = until
		}
	}
	f.sent = append(f.sent, "mute|"+jid+"|"+length)
	return until, nil
}

func TestMuteText(t *testing.T) {
	at := time.Date(2026, 11, 18, 5, 0, 0, 0, time.Local).UnixMilli()
	for _, c := range []struct {
		until int64
		only  bool
		want  string
	}{
		{0, false, ""},
		{0, true, "in Positron only"},
		{at, false, "until 2026-11-18 05:00"},
		{time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), false, "always"},
	} {
		if got := muteText(c.until, c.only); got != c.want {
			t.Errorf("muteText(%d, %v) = %q, want %q", c.until, c.only, got, c.want)
		}
	}
}

func TestMuteTool(t *testing.T) {
	s, fb, hs := testServer(t)
	cs := connect(t, s, hs.URL)
	out, isErr := call(t, cs, "mute_whatsapp_chat", map[string]any{"chat": "souza", "for": "always"})
	if isErr || out != "Ana Souza is muted always." || fb.sent[len(fb.sent)-1] != "mute|5551999994412@s.whatsapp.net|always" {
		t.Fatalf("mute: %s %v", out, fb.sent)
	}
	out, _ = call(t, cs, "list_chats", map[string]any{"text": "souza"})
	if !strings.Contains(out, `"muted": "always"`) {
		t.Fatalf("list_chats: %s", out)
	}
	if out, _ = call(t, cs, "mute_whatsapp_chat", map[string]any{"chat": "souza", "for": "8h"}); out != "Ana Souza is muted until 2026-11-18 05:00." {
		t.Fatalf("8h: %s", out)
	}
	if out, _ = call(t, cs, "mute_whatsapp_chat", map[string]any{"chat": "souza", "for": "off"}); out != "Ana Souza is unmuted." {
		t.Fatalf("off: %s", out)
	}
	if out, _ = call(t, cs, "list_chats", map[string]any{"text": "souza"}); strings.Contains(out, "muted") {
		t.Fatalf("still muted: %s", out)
	}
}
