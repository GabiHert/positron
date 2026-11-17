package backend

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/engine"
	"github.com/GabiHert/positron/internal/media"
	"github.com/GabiHert/positron/internal/store"
)

// fakeSpeech is a speech model that "hears" the same sentence every time, slowly.
type fakeSpeech struct {
	ready bool
	runs  atomic.Int32
	delay time.Duration
}

func (f *fakeSpeech) Ready(domain.Language) (bool, string) {
	if !f.ready {
		return false, "model not installed"
	}
	return true, ""
}
func (f *fakeSpeech) Transcribe(context.Context, []float32, int, domain.Language) (string, error) {
	f.runs.Add(1)
	time.Sleep(f.delay)
	return "oi, tudo bem?", nil
}

// silentWAV writes a short 16 kHz mono WAV file.
func silentWAV(t *testing.T, path string) {
	t.Helper()
	const n = 1600
	b := make([]byte, 44+n*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+n*2))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], n*2)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

type voiceRig struct {
	b      *Backend
	st     *store.Store
	speech *fakeSpeech
	mu     sync.Mutex
	events []app.ChatMessageDTO
}

func (r *voiceRig) seen() []app.ChatMessageDTO {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]app.ChatMessageDTO(nil), r.events...)
}

const voiceChat = "ana@s.whatsapp.net"

func newVoiceRig(t *testing.T) *voiceRig {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := &voiceRig{st: st, speech: &fakeSpeech{ready: true, delay: 100 * time.Millisecond}}
	r.b = &Backend{d: Deps{Store: st, Media: media.Store{Dir: dir}, Emit: func(ev string, data any) {
		if m, ok := data.(app.MessageEventDTO); ok && ev == app.EventMessage {
			r.mu.Lock()
			r.events = append(r.events, m.Message)
			r.mu.Unlock()
		}
	}}}
	r.b.d.Engine = engine.New(engine.Deps{Store: st, Transcriber: media.Transcriber{Engine: r.speech, TmpDir: dir}})
	t.Cleanup(r.b.d.Engine.Close)
	voice := filepath.Join(dir, "v1.wav")
	silentWAV(t, voice)
	_ = st.SaveChat(domain.Chat{JID: voiceChat, Name: "Ana"})
	for _, m := range []domain.Message{
		{ID: "v1", ChatJID: voiceChat, Kind: domain.KindVoice, MediaPath: voice, Mime: "audio/wav", Seconds: 1, At: time.Now()},
		{ID: "t1", ChatJID: voiceChat, Kind: domain.KindText, Text: "oi"},
		{ID: "once", ChatJID: voiceChat, Kind: domain.KindVoice, ViewOnce: true, MediaPath: voice},
	} {
		if err := st.SaveMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestTranscribeVoiceRunsOnceAndKeepsTheText(t *testing.T) {
	r := newVoiceRig(t)
	var wg sync.WaitGroup
	texts := make([]string, 3)
	for i := range texts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, err := r.b.TranscribeVoice(voiceChat, "v1")
			if err != nil {
				t.Error(err)
			}
			texts[i] = text
		}()
	}
	wg.Wait()
	if r.speech.runs.Load() != 1 {
		t.Fatalf("the model ran %d times for one voice note", r.speech.runs.Load())
	}
	for _, text := range texts {
		if text != "oi, tudo bem?" {
			t.Fatalf("texts %q", texts)
		}
	}
	if m, _ := r.st.FindMessage(voiceChat, "v1"); m.Transcript != "oi, tudo bem?" {
		t.Fatalf("stored %q", m.Transcript)
	}
	// The chat view saw it transcribing, then the text.
	ev := r.seen()
	if len(ev) < 2 || !ev[0].Transcribing || ev[len(ev)-1].Transcribing || ev[len(ev)-1].Transcript != "oi, tudo bem?" {
		t.Fatalf("events %+v", ev)
	}
	// Asked again later: the stored text, no new run.
	if text, err := r.b.TranscribeVoice(voiceChat, "v1"); err != nil || text != "oi, tudo bem?" || r.speech.runs.Load() != 1 {
		t.Fatalf("again: %q %v, runs %d", text, err, r.speech.runs.Load())
	}
}

func TestTranscribeVoiceRefuses(t *testing.T) {
	r := newVoiceRig(t)
	for id, want := range map[string]string{"t1": "only voice notes", "once": "view-once", "nope": "gone"} {
		if _, err := r.b.TranscribeVoice(voiceChat, id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %v", id, want, err)
		}
	}
	// Without the language's model it says how to get it, and the message is not left transcribing.
	r.speech.ready = false
	if _, err := r.b.TranscribeVoice(voiceChat, "v1"); err == nil || !strings.Contains(err.Error(), "Download the model in Settings") {
		t.Fatalf("no model: %v", err)
	}
	if ev := r.seen(); ev[len(ev)-1].Transcribing {
		t.Fatal("still shown as transcribing after the error")
	}
}

func TestAutoTranscribe(t *testing.T) {
	r := newVoiceRig(t)
	voice := domain.Message{ID: "v1", ChatJID: voiceChat, Kind: domain.KindVoice, At: time.Now()}
	if r.b.wantsTranscript(voice) {
		t.Fatal("the default leaves chats without an agent alone, as before")
	}
	s := r.st.Settings()
	s.AutoTranscribe = domain.TranscribeAll
	_ = r.st.SaveSettings(s)
	if !r.b.wantsTranscript(voice) {
		t.Fatal("all: a new voice note is transcribed")
	}
	mine, old, once, text := voice, voice, voice, voice
	mine.FromMe, old.History, once.ViewOnce, text.Kind = true, true, true, domain.KindText
	done := voice
	done.Transcript = "já tem"
	for name, m := range map[string]domain.Message{"mine": mine, "history": old, "view once": once, "text": text, "transcribed": done} {
		if r.b.wantsTranscript(m) {
			t.Errorf("%s should not be transcribed", name)
		}
	}
	r.speech.ready = false
	if r.b.wantsTranscript(voice) {
		t.Error("no model installed: nothing to run")
	}
	r.speech.ready = true

	// An agent that reads voice notes transcribes them itself: no second run.
	a := domain.NewAgent(domain.DefaultSettings().Defaults)
	a.ID, a.Chats = "a1", []string{voiceChat}
	_ = r.st.SaveAgent(a)
	if r.b.wantsTranscript(voice) {
		t.Error("the agent's chat is transcribed by the agent")
	}
	a.TranscribeVoice = false
	_ = r.st.SaveAgent(a)
	if !r.b.wantsTranscript(voice) {
		t.Error("an agent that doesn't read voice notes leaves it to the setting")
	}

	// A new voice note is transcribed in the background.
	r.b.OnMessage(voice, "new")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if m, _ := r.st.FindMessage(voiceChat, "v1"); m.Transcript != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not transcribed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTranscribeSetting(t *testing.T) {
	for in, want := range map[string]string{"all": "all", "agents": "agents", "": "agents", "weird": "agents"} {
		if got := transcribeSetting(in); got != want {
			t.Errorf("%q = %q", in, got)
		}
	}
	if domain.DefaultSettings().AutoTranscribe != domain.TranscribeAgents {
		t.Error("default")
	}
}
