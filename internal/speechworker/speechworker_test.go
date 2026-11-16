package speechworker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/domain"
)

// The test binary doubles as the helper: with SPEECHWORKER_HELPER set it serves a fake model.
func TestMain(m *testing.M) {
	switch os.Getenv("SPEECHWORKER_HELPER") {
	case "echo":
		err := Serve(os.Stdin, os.Stdout, func(_ context.Context, s []float32, rate int, lang domain.Language) (string, error) {
			if s[0] < 0 {
				return "", errors.New("no speech found")
			}
			if s[0] == 9 {
				time.Sleep(time.Minute) // a transcription that hangs
			}
			return fmt.Sprintf("%d samples at %d Hz in %s, first %.2f, pid %d", len(s), rate, lang, s[0], os.Getpid()), nil
		})
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "crash":
		os.Exit(3)
	}
	os.Exit(m.Run())
}

func client(mode string, idle time.Duration) *Client {
	return &Client{Idle: idle, Command: func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), "SPEECHWORKER_HELPER="+mode)
		return cmd
	}}
}

func TestTranscribeReusesTheHelperAndItExitsWhenIdle(t *testing.T) {
	c := client("echo", 300*time.Millisecond)
	defer c.Unload()
	ctx := context.Background()
	a, err := c.Transcribe(ctx, []float32{0.25, 0.5, -1}, 16000, domain.LangPTBR)
	if err != nil || !strings.HasPrefix(a, "3 samples at 16000 Hz in pt-BR, first 0.25") {
		t.Fatalf("got %q, %v", a, err)
	}
	b, err := c.Transcribe(ctx, make([]float32, 48000), 16000, domain.LangEN)
	if err != nil || !strings.HasPrefix(b, "48000 samples at 16000 Hz in en") {
		t.Fatalf("got %q, %v", b, err)
	}
	pid := func(s string) string { return s[strings.LastIndex(s, " ")+1:] }
	if pid(a) != pid(b) {
		t.Fatalf("a second helper started: %s then %s", pid(a), pid(b))
	}
	if !c.Running() {
		t.Fatal("the helper should still be up between notes")
	}
	deadline := time.Now().Add(3 * time.Second)
	for c.Running() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if c.Running() {
		t.Fatal("the helper did not exit after being idle")
	}
	// The next note starts a fresh helper.
	d, err := c.Transcribe(ctx, []float32{1}, 8000, domain.LangEN)
	if err != nil || pid(d) == pid(a) {
		t.Fatalf("got %q, %v", d, err)
	}
}

func TestModelErrorsComeBackAndTheHelperStays(t *testing.T) {
	c := client("echo", time.Minute)
	defer c.Unload()
	if _, err := c.Transcribe(context.Background(), []float32{-1}, 16000, domain.LangEN); err == nil || err.Error() != "no speech found" {
		t.Fatalf("err %v", err)
	}
	if !c.Running() {
		t.Fatal("a model error should not stop the helper")
	}
}

func TestACrashedHelperIsAnErrorAndTheNextCallStartsAgain(t *testing.T) {
	c := client("crash", time.Minute)
	if _, err := c.Transcribe(context.Background(), []float32{1}, 16000, domain.LangEN); err == nil {
		t.Fatal("want an error from a crashed helper")
	}
	if c.Running() {
		t.Fatal("a crashed helper is still counted as running")
	}
	c.Command = client("echo", 0).Command
	defer c.Unload()
	if _, err := c.Transcribe(context.Background(), []float32{1}, 16000, domain.LangEN); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
}

func TestCancelKillsAStuckHelper(t *testing.T) {
	c := client("echo", time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Transcribe(ctx, []float32{9}, 16000, domain.LangEN)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second || c.Running() {
		t.Fatalf("err %v after %s, running %v", err, time.Since(start), c.Running())
	}
}

func TestEmptyAndTooLongAudio(t *testing.T) {
	c := client("echo", time.Minute)
	if _, err := c.Transcribe(context.Background(), nil, 16000, domain.LangEN); err == nil {
		t.Fatal("want an error for empty audio")
	}
	if c.Running() {
		t.Fatal("empty audio should not start the helper")
	}
}
