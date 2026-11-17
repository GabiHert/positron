package backend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/media"
	"github.com/GabiHert/positron/internal/store"
)

func TestMessageFile(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	dir := t.TempDir()
	photo := filepath.Join(dir, "p1.jpg")
	_ = os.WriteFile(photo, []byte("jpeg"), 0o600)
	b := &Backend{d: Deps{Store: st, Media: media.Store{Dir: dir}}}
	chat := "ana@s.whatsapp.net"
	for _, m := range []domain.Message{
		{ID: "p1", ChatJID: chat, Kind: domain.KindImage, MediaPath: photo, Mime: "image/jpeg"},
		{ID: "o1", ChatJID: chat, Kind: domain.KindImage, ViewOnce: true, MediaPath: photo},
		{ID: "t1", ChatJID: chat, Kind: domain.KindText, Text: "oi"},
		{ID: "old", ChatJID: chat, Kind: domain.KindDocument, FileName: "a.pdf"},
	} {
		if err := st.SaveMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if m, p, err := b.MessageFile(ctx, chat, "p1"); err != nil || p != photo || m.ID != "p1" {
		t.Fatalf("p1: %q %v", p, err)
	}
	for id, want := range map[string]string{"o1": "view-once", "t1": "no photo", "old": "only be opened on your phone", "nope": "gone"} {
		if _, p, err := b.MessageFile(ctx, chat, id); err == nil || p != "" || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %q %v", id, want, p, err)
		}
	}
}

func TestTranscriptLanguage(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_ = st.SaveAgent(domain.Agent{ID: "a1", Name: "A", Chats: []string{"en@s.whatsapp.net"}, Language: domain.LangEN})
	b := &Backend{d: Deps{Store: st}}
	for _, c := range []struct {
		chat, asked string
		want        domain.Language
	}{
		{"en@s.whatsapp.net", "pt-BR", domain.LangEN}, // the chat's agent wins
		{"x@s.whatsapp.net", "EN", domain.LangEN},
		{"x@s.whatsapp.net", "", domain.LangPTBR},
		{"x@s.whatsapp.net", "klingon", domain.LangPTBR},
	} {
		if got := b.transcriptLanguage(c.chat, c.asked); got != c.want {
			t.Errorf("%s %q: got %s, want %s", c.chat, c.asked, got, c.want)
		}
	}
}
