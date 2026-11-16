// Package speechworker runs Whisper in a helper process. Loading the model takes the process to
// about 2.5 GB, and sherpa-onnx does not hand that memory back after an unload: each load and
// unload in the app's own process left more behind. A helper process gives every byte back when
// it exits, so the app starts one for the first voice note, reuses it while notes keep coming
// and lets it exit when it has been idle for a while.
package speechworker

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/domain"
)

// Flag starts the binary as the speech helper: positron --speech-worker <models dir>.
const Flag = "--speech-worker"

// maxSamples bounds one request: an hour of 16 kHz audio.
const maxSamples = 16000 * 3600

type request struct {
	Lang domain.Language `json:"lang"`
	Rate int             `json:"rate"`
	N    int             `json:"n"` // float32 samples that follow, little endian
}

type response struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

// TranscribeFunc turns samples into text inside the helper.
type TranscribeFunc func(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error)

// Serve answers requests from r on w until r ends. It runs in the helper process.
func Serve(r io.Reader, w io.Writer, transcribe TranscribeFunc) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil || req.N < 0 || req.N > maxSamples {
			return fmt.Errorf("speech worker: bad request %q", line)
		}
		raw := make([]byte, 4*req.N)
		if _, err := io.ReadFull(br, raw); err != nil {
			return err
		}
		samples := make([]float32, req.N)
		for i := range samples {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
		}
		raw = nil
		var resp response
		resp.Text, err = transcribe(context.Background(), samples, req.Rate, req.Lang)
		if err != nil {
			resp.Error = err.Error()
		}
		b, _ := json.Marshal(resp)
		if _, err := bw.Write(append(b, '\n')); err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
}

// Client sends transcriptions to a helper process that it starts when needed. It satisfies
// port.SpeechEngine.
type Client struct {
	// Command builds the helper's command line.
	Command func() *exec.Cmd
	// ReadyFn reports whether the model files for lang are on disk; it must not load the model.
	ReadyFn func(lang domain.Language) (bool, string)
	// Idle is how long the helper stays after its last transcription.
	Idle time.Duration

	mu    sync.Mutex // one transcription at a time; guards the fields below
	cmd   *exec.Cmd
	in    io.WriteCloser
	out   *bufio.Reader
	timer *time.Timer
	gen   int // which helper the idle timer belongs to
}

// Ready reports whether lang can be transcribed.
func (c *Client) Ready(lang domain.Language) (bool, string) {
	if c.ReadyFn == nil {
		return true, ""
	}
	return c.ReadyFn(lang)
}

// Transcribe sends samples to the helper, starting it if it is not running.
func (c *Client) Transcribe(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(samples) == 0 {
		return "", errors.New("empty audio")
	}
	if len(samples) > maxSamples {
		return "", errors.New("the audio is too long to transcribe")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil {
		if err := c.startLocked(); err != nil {
			return "", err
		}
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	text, modelErr, err := c.roundTrip(ctx, samples, sampleRate, lang)
	if err != nil {
		// The helper is in an unknown state (killed, crashed, or cut off mid-answer).
		c.stopLocked()
		return "", err
	}
	c.armIdleLocked()
	if modelErr != "" {
		return "", errors.New(modelErr)
	}
	return text, nil
}

// roundTrip returns the text, or the model's own error, or err when the helper itself failed.
func (c *Client) roundTrip(ctx context.Context, samples []float32, rate int, lang domain.Language) (text, modelErr string, err error) {
	type result struct {
		resp response
		err  error
	}
	done := make(chan result, 1)
	in, out := c.in, c.out
	go func() {
		hdr, _ := json.Marshal(request{Lang: lang, Rate: rate, N: len(samples)})
		buf := make([]byte, 0, len(hdr)+1+4*len(samples))
		buf = append(append(buf, hdr...), '\n')
		for _, s := range samples {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(s))
		}
		if _, err := in.Write(buf); err != nil {
			done <- result{err: fmt.Errorf("the speech helper stopped: %w", err)}
			return
		}
		line, err := out.ReadBytes('\n')
		if err != nil {
			done <- result{err: fmt.Errorf("the speech helper stopped: %w", err)}
			return
		}
		var r response
		if err := json.Unmarshal(line, &r); err != nil {
			done <- result{err: fmt.Errorf("the speech helper sent an answer Positron could not read: %q", line)}
			return
		}
		done <- result{resp: r}
	}()
	select {
	case <-ctx.Done():
		c.stopLocked() // unblocks the goroutine
		<-done
		return "", "", ctx.Err()
	case r := <-done:
		return r.resp.Text, r.resp.Error, r.err
	}
}

func (c *Client) startLocked() error {
	cmd := c.Command()
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start the speech helper: %w", err)
	}
	c.cmd, c.in, c.out = cmd, in, bufio.NewReader(out)
	c.gen++
	return nil
}

func (c *Client) armIdleLocked() {
	idle := c.Idle
	if idle <= 0 {
		idle = 3 * time.Minute
	}
	gen := c.gen
	c.timer = time.AfterFunc(idle, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.gen == gen {
			c.stopLocked()
		}
	})
}

// stopLocked ends the helper: closing its input lets it exit; a stuck one is killed.
func (c *Client) stopLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if c.cmd == nil {
		return
	}
	cmd := c.cmd
	_ = c.in.Close()
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}
	c.cmd, c.in, c.out = nil, nil, nil
	c.gen++
}

// Running reports whether a helper process is up.
func (c *Client) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cmd != nil
}

// Unload stops the helper, which gives all of the model's memory back.
func (c *Client) Unload() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}
