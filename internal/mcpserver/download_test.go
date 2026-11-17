package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/app"
)

// mediaFake is the stored chat the download and export tools read.
type mediaFake struct {
	msgs        []app.ChatMessageDTO // oldest first
	files       map[string]string    // message id -> file on "this Mac"
	errs        map[string]error
	transcripts map[string]string
	fetched     []string
	transcribed []string
}

func (f *fakeBackend) ListMessages(chat, before string, limit int) (app.MessagePageDTO, error) {
	// newest first, paged by index like the store's cursor
	var all []app.ChatMessageDTO
	for i := len(f.media.msgs) - 1; i >= 0; i-- {
		if f.media.msgs[i].ChatJID == chat {
			all = append(all, f.media.msgs[i])
		}
	}
	start := 0
	if before != "" {
		fmt.Sscan(before, &start)
	}
	end := min(start+limit, len(all))
	page := app.MessagePageDTO{Messages: all[start:end]}
	if end < len(all) {
		page.Cursor = fmt.Sprint(end)
	}
	return page, nil
}

func (f *fakeBackend) MessageFile(_ context.Context, chat, id string) (app.ChatMessageDTO, string, error) {
	for _, m := range f.media.msgs {
		if m.ChatJID == chat && m.ID == id {
			if err := f.media.errs[id]; err != nil {
				return m, "", err
			}
			if m.ViewOnce {
				return m, "", errors.New("view once")
			}
			f.media.fetched = append(f.media.fetched, id)
			return m, f.media.files[id], nil
		}
	}
	return app.ChatMessageDTO{}, "", errors.New("that message is gone")
}

func (f *fakeBackend) TranscribeMessage(_ context.Context, chat, id, lang string) (string, error) {
	f.media.transcribed = append(f.media.transcribed, id+"|"+lang)
	if t, ok := f.media.transcripts[id]; ok {
		return t, nil
	}
	return "", errors.New("voice notes in English can't be transcribed yet")
}

const ana = "5551999994412@s.whatsapp.net"

// mediaServer is a server whose home folder is a temp folder with a stored chat in it.
func mediaServer(t *testing.T) (*Server, *fakeBackend, string, string) {
	t.Helper()
	home := t.TempDir()
	src := t.TempDir() // Positron's own media folder
	write := func(name, body string) string {
		p := filepath.Join(src, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	day := time.Date(2026, 11, 14, 9, 0, 0, 0, time.Local)
	at := func(d, h int) int64 { return day.AddDate(0, 0, d).Add(time.Duration(h) * time.Hour).UnixMilli() }
	fb := newFake()
	fb.media = mediaFake{
		msgs: []app.ChatMessageDTO{
			{ID: "t1", ChatJID: ana, SenderName: "Ana Souza", At: at(0, 0), Kind: "text", Text: "Bom dia *Gabriel*"},
			{ID: "p1", ChatJID: ana, SenderName: "Ana Souza", At: at(0, 1), Kind: "image", Text: "a praia",
				Media: &app.MediaDTO{Mime: "image/jpeg", Size: 2048, URL: "/media/p1.jpg"}},
			{ID: "d1", ChatJID: ana, FromMe: true, At: at(1, 0), Kind: "document",
				Media: &app.MediaDTO{Mime: "application/pdf", FileName: "Contrato: v2.pdf", Size: 3 << 20, Downloadable: true}},
			{ID: "v1", ChatJID: ana, SenderName: "Ana Souza", At: at(1, 2), Kind: "voice",
				Media: &app.MediaDTO{Mime: "audio/ogg; codecs=opus", Seconds: 75, Downloadable: true}},
			{ID: "o1", ChatJID: ana, SenderName: "Ana Souza", At: at(2, 0), Kind: "image", ViewOnce: true,
				Media: &app.MediaDTO{Mime: "image/jpeg", Downloadable: true}},
			{ID: "x1", ChatJID: ana, SenderName: "Ana Souza", At: at(2, 1), Kind: "video",
				Media: &app.MediaDTO{Mime: "video/mp4"}},
			{ID: "t2", ChatJID: ana, SenderName: "Ana Souza", At: at(2, 2), Kind: "text", Text: "Até amanhã"},
		},
		files: map[string]string{
			"p1": write("p1.jpg", "jpeg"),
			"d1": write("d1.pdf", "pdf"),
			"v1": write("v1.ogg", "ogg"),
		},
		errs:        map[string]error{"x1": errors.New("WhatsApp no longer has this file (WhatsApp deletes media from its servers after a while); it may still be on the phone")},
		transcripts: map[string]string{"v1": "Chego às sete"},
	}
	s := New(fb, Options{Home: t.TempDir(), Port: "0", UserHome: home})
	return s, fb, home, src
}

func TestDownloadWhatsAppMedia(t *testing.T) {
	s, fb, home, _ := mediaServer(t)
	hs := httptest.NewServer(s.Handler())
	t.Cleanup(hs.Close)
	cs := connect(t, s, hs.URL)
	real, _ := realPath(home)

	var got savedOut
	out, isErr := call(t, cs, "download_whatsapp_media", map[string]any{"chat": "Ana Souza", "message_id": "p1"})
	if isErr || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("%v %s", isErr, out)
	}
	wantName := "WhatsApp photo " + time.UnixMilli(fb.media.msgs[1].At).Format("2006-01-02 at 15.04.05") + ".jpg"
	if got.Path != filepath.Join(real, "Downloads", "Positron", wantName) || got.Mime != "image/jpeg" || got.Size != 4 || got.Caption != "a praia" {
		t.Fatalf("got %+v", got)
	}
	// Again: a second file, never a replaced one.
	out, _ = call(t, cs, "download_whatsapp_media", map[string]any{"chat": "Ana Souza", "message_id": "p1"})
	if !strings.Contains(out, strings.TrimSuffix(wantName, ".jpg")+" (2).jpg") {
		t.Fatalf("second download: %s", out)
	}

	// A document keeps its own name (made safe), in the folder asked for.
	out, isErr = call(t, cs, "download_whatsapp_media", map[string]any{"chat": ana, "message_id": "d1", "folder": "~/Documents/contratos"})
	if isErr || !strings.Contains(out, filepath.Join(real, "Documents", "contratos", "Contrato- v2.pdf")) || !strings.Contains(out, `"mime": "application/pdf"`) {
		t.Fatalf("%v %s", isErr, out)
	}

	// A voice note with its transcript, in the language asked for when the chat has no agent.
	out, isErr = call(t, cs, "download_whatsapp_media", map[string]any{"chat": ana, "message_id": "v1", "transcript": true, "language": "pt-BR"})
	if isErr || !strings.Contains(out, `"transcript": "Chego às sete"`) || !strings.Contains(out, `"mime": "audio/ogg"`) {
		t.Fatalf("%v %s", isErr, out)
	}
	if fb.media.transcribed[0] != "v1|pt-BR" {
		t.Fatalf("transcribed %v", fb.media.transcribed)
	}

	// Expired media says so; view-once is refused; nothing is written for them.
	if out, isErr = call(t, cs, "download_whatsapp_media", map[string]any{"chat": ana, "message_id": "x1"}); !isErr || !strings.Contains(out, "no longer has this file") {
		t.Fatalf("expired: %v %s", isErr, out)
	}
	if _, isErr = call(t, cs, "download_whatsapp_media", map[string]any{"chat": ana, "message_id": "o1"}); !isErr {
		t.Fatal("view once should be refused")
	}
	if n, _ := filepath.Glob(filepath.Join(real, "Downloads", "Positron", "*")); len(n) != 3 {
		t.Fatalf("files in Downloads/Positron: %v", n)
	}
}

func TestWritesStayInsideTheHomeFolder(t *testing.T) {
	s, _, home, _ := mediaServer(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, "sneaky")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{outside, "/tmp", "relative/folder", "~/../", filepath.Join(home, "..", "x"), "~/sneaky", "~/sneaky/deeper"} {
		if _, err := s.folder(dir); err == nil {
			t.Errorf("%q: want a refusal", dir)
		}
	}
	for _, dir := range []string{"", "~", "~/Desktop/new/folder", filepath.Join(home, "Pictures")} {
		got, err := s.folder(dir)
		if err != nil {
			t.Errorf("%q: %v", dir, err)
			continue
		}
		if st, err := os.Stat(got); err != nil || !st.IsDir() {
			t.Errorf("%q: folder not made", dir)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside the home folder: %v", entries)
	}
}

func TestExportWhatsAppChat(t *testing.T) {
	s, fb, home, _ := mediaServer(t)
	hs := httptest.NewServer(s.Handler())
	t.Cleanup(hs.Close)
	cs := connect(t, s, hs.URL)

	// Markdown, a range by days, with media.
	out, isErr := call(t, cs, "export_whatsapp_chat", map[string]any{"chat": "Ana Souza", "since": "2026-11-14", "until": "2026-11-15", "include_media": true, "folder": "~/Exports"})
	var res exportResult
	if isErr || json.Unmarshal([]byte(out), &res) != nil {
		t.Fatalf("%v %s", isErr, out)
	}
	if res.Messages != 4 || res.MediaSaved != 3 || res.MediaMissing != 0 || !strings.HasSuffix(res.Path, "WhatsApp Ana Souza 2026-11-14 to 2026-11-15.md") {
		t.Fatalf("got %+v", res)
	}
	b, _ := os.ReadFile(res.Path)
	md := string(b)
	for _, want := range []string{
		"# WhatsApp: Ana Souza", "## 2026-11-14", "## 2026-11-15",
		"**09:00** Ana Souza: Bom dia *Gabriel*",
		"Ana Souza: [photo, 2 KB](<WhatsApp Ana Souza 2026-11-14 to 2026-11-15 media/WhatsApp photo 2026-11-14 at 10.00.00.jpg>) a praia",
		"owner: [document Contrato: v2.pdf, 3.0 MB](<WhatsApp Ana Souza 2026-11-14 to 2026-11-15 media/Contrato- v2.pdf>)",
		"[voice note, 1:15](",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "Até amanhã") {
		t.Error("a message after until was exported")
	}
	if media, _ := os.ReadDir(res.MediaFolder); len(media) != 3 {
		t.Fatalf("media folder: %v", media)
	}

	// JSON, the last 3 messages, no media: view-once and expired media are listed, not fetched.
	fb.media.fetched = nil
	out, isErr = call(t, cs, "export_whatsapp_chat", map[string]any{"chat": ana, "last": 3, "format": "json"})
	res = exportResult{}
	if isErr || json.Unmarshal([]byte(out), &res) != nil || res.Messages != 3 || len(fb.media.fetched) != 0 {
		t.Fatalf("%v %s fetched %v", isErr, out, fb.media.fetched)
	}
	real, _ := realPath(home)
	if filepath.Dir(res.Path) != filepath.Join(real, "Downloads", "Positron") {
		t.Fatalf("default folder: %s", res.Path)
	}
	var doc struct {
		Chat     string      `json:"chat"`
		Messages []exportRow `json:"messages"`
	}
	b, _ = os.ReadFile(res.Path)
	if err := json.Unmarshal(b, &doc); err != nil || doc.Chat != "Ana Souza" || len(doc.Messages) != 3 {
		t.Fatalf("%v %+v", err, doc)
	}
	if doc.Messages[0].ID != "o1" || doc.Messages[0].Media.Available != "view once" || doc.Messages[1].Media.Available != "only on the phone" || doc.Messages[2].Text != "Até amanhã" {
		t.Fatalf("messages %+v", doc.Messages)
	}

	// With media, the view-once and expired ones are noted, the rest still exported.
	out, _ = call(t, cs, "export_whatsapp_chat", map[string]any{"chat": ana, "since": "2026-11-16", "include_media": true})
	res = exportResult{}
	if json.Unmarshal([]byte(out), &res) != nil || res.MediaMissing != 2 || res.MediaSaved != 0 || !strings.HasSuffix(res.Path, "WhatsApp Ana Souza 2026-11-16.md") {
		t.Fatalf("%s", out)
	}
	b, _ = os.ReadFile(res.Path)
	if !strings.Contains(string(b), "(not saved: WhatsApp no longer has this file") || !strings.Contains(string(b), "(not saved: view once") {
		t.Fatalf("%s", b)
	}

	for args, want := range map[string]string{
		`{"since":"yesterday"}`:                       "is not a date",
		`{"since":"2026-11-15","until":"2026-11-14"}`: "until is before since",
		`{"format":"pdf"}`:                            "markdown or json",
		`{"folder":"/etc"}`:                           "outside your home folder",
		`{"since":"2020-01-01","until":"2020-01-02"}`: "no messages",
		`{"last":50000}`:                              "at most",
	} {
		in := map[string]any{"chat": ana}
		_ = json.Unmarshal([]byte(args), &in)
		if out, isErr := call(t, cs, "export_whatsapp_chat", in); !isErr || !strings.Contains(out, want) {
			t.Errorf("%s: want %q, got %v %s", args, want, isErr, out)
		}
	}
}

func TestReadAndSearchDescribeMedia(t *testing.T) {
	_, fb, _, _ := mediaServer(t)
	for id, want := range map[string]mediaInfo{
		"p1": {Type: "photo", Mime: "image/jpeg", Size: 2048, Available: "on this Mac"},
		"d1": {Type: "document", Mime: "application/pdf", Size: 3 << 20, FileName: "Contrato: v2.pdf", Available: "can be fetched from WhatsApp"},
		"v1": {Type: "voice note", Mime: "audio/ogg", Seconds: 75, Available: "can be fetched from WhatsApp"},
		"o1": {Type: "photo", Mime: "image/jpeg", Available: "view once"},
		"x1": {Type: "video", Mime: "video/mp4", Available: "only on the phone"},
	} {
		for _, m := range fb.media.msgs {
			if m.ID == id {
				if got := toMsg(m).Media; got == nil || *got != want {
					t.Errorf("%s: got %+v, want %+v", id, got, want)
				}
			}
		}
	}
	if toMsg(fb.media.msgs[0]).Media != nil {
		t.Error("a text message has no media")
	}
}
