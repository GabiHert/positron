package backend

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/domain"
)

// transcribeTimeout bounds one voice note: downloading it and running the model.
const transcribeTimeout = 5 * time.Minute

// voiceJobs are the voice notes being transcribed, so a second click or the automatic
// transcription waits for the first run instead of starting another.
type voiceJobs struct {
	mu   sync.Mutex
	jobs map[string]*voiceJob
}

type voiceJob struct {
	done chan struct{}
	text string
	err  error
}

func voiceKey(chat, id string) string { return chat + "/" + id }

func (v *voiceJobs) running(chat, id string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.jobs[voiceKey(chat, id)]
	return ok
}

// start returns the job for the voice note and whether the caller has to run it.
func (v *voiceJobs) start(chat, id string) (*voiceJob, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if j, ok := v.jobs[voiceKey(chat, id)]; ok {
		return j, false
	}
	if v.jobs == nil {
		v.jobs = map[string]*voiceJob{}
	}
	j := &voiceJob{done: make(chan struct{})}
	v.jobs[voiceKey(chat, id)] = j
	return j, true
}

func (v *voiceJobs) finish(chat, id string, j *voiceJob) {
	v.mu.Lock()
	delete(v.jobs, voiceKey(chat, id))
	v.mu.Unlock()
	close(j.done)
}

// TranscribeVoice transcribes a voice note or audio message for the chat view and keeps the
// text. One that already has a transcript returns it at once; one already being transcribed
// waits for that run. While it runs the message shows as transcribing.
func (b *Backend) TranscribeVoice(chat, id string) (string, error) {
	m, err := b.d.Store.FindMessage(chat, id)
	if err != nil {
		return "", errors.New("that message is gone")
	}
	if m.Transcript != "" {
		return m.Transcript, nil
	}
	if m.Kind != domain.KindVoice && m.Kind != domain.KindAudio {
		return "", errors.New("only voice notes and audio have transcripts")
	}
	if m.ViewOnce {
		return "", errors.New("view-once voice notes are not transcribed")
	}
	j, mine := b.voice.start(chat, id)
	if !mine {
		<-j.done
		return j.text, j.err
	}
	b.OnMessage(m, "update") // shows "Transcribing…"
	ctx, cancel := context.WithTimeout(context.Background(), transcribeTimeout)
	defer cancel()
	j.text, j.err = b.TranscribeMessage(ctx, chat, id, "")
	b.voice.finish(chat, id, j)
	if j.err != nil {
		if m, err := b.d.Store.FindMessage(chat, id); err == nil {
			b.OnMessage(m, "update") // no longer transcribing
		}
	}
	// On success TranscribeMessage already reported the message with its transcript.
	return j.text, j.err
}

// transcribeSetting keeps AutoTranscribe to its known values; anything else is the default.
func transcribeSetting(v string) string {
	if v == domain.TranscribeAll {
		return v
	}
	return domain.TranscribeAgents
}

// wantsTranscript says whether a new message is a voice note Positron transcribes on its own
// with AutoTranscribe "all": someone else's, live (not history), not view once, and in a chat
// whose agent doesn't transcribe it already.
func (b *Backend) wantsTranscript(m domain.Message) bool {
	if m.Kind != domain.KindVoice && m.Kind != domain.KindAudio {
		return false
	}
	if m.FromMe || m.History || m.ViewOnce || m.Revoked || m.Transcript != "" {
		return false
	}
	if transcribeSetting(b.d.Store.Settings().AutoTranscribe) != domain.TranscribeAll {
		return false
	}
	if b.d.Engine != nil && b.d.Engine.TranscribesVoice(m.ChatJID) {
		return false // the agent transcribes it as part of its run
	}
	// Without the language's model it would only fail; the Transcribe button says why.
	return b.d.Engine != nil && b.d.Engine.CanTranscribe(b.transcriptLanguage(m.ChatJID, ""))
}

// autoTranscribe transcribes a new voice note in the background when the setting asks for it.
func (b *Backend) autoTranscribe(m domain.Message) {
	if !b.wantsTranscript(m) {
		return
	}
	go func() {
		if _, err := b.TranscribeVoice(m.ChatJID, m.ID); err != nil {
			log.Printf("transcribe %s: %v", m.ID, err)
		}
	}()
}
