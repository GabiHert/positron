package system

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPathResolvesServedURLsAndNothingElse(t *testing.T) {
	media, repo := t.TempDir(), t.TempDir()
	write := func(p string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	photo := write(filepath.Join(media, "chat@s.whatsapp.net", "a b.jpg"))
	chart := write(filepath.Join(repo, "out", "chart.png"))
	write(filepath.Join(repo, ".git", "config"))
	write(filepath.Join(filepath.Dir(media), "secret.txt"))
	h := FileHandler{MediaDir: media, RepoDir: func(id string) (string, bool) { return repo, id == "ag1" }}

	for _, tc := range []struct {
		src, want string
	}{
		{"/media/chat@s.whatsapp.net/a%20b.jpg", photo},
		{"wails://wails/media/chat@s.whatsapp.net/a%20b.jpg", photo},
		{"http://wails.localhost/media/chat@s.whatsapp.net/a%20b.jpg?x=1", photo},
		{"/repo/ag1/out/chart.png", chart},
		{"/repo/ag2/out/chart.png", ""},
		{"/repo/ag1/.git/config", ""},
		{"/media/../secret.txt", ""},
		{"/media/%2e%2e/secret.txt", ""},
		{"/media/chat@s.whatsapp.net", ""}, // a folder
		{"/media/missing.jpg", ""},
		{"/etc/hosts", ""},
		{"data:image/png;base64,AAAA", ""},
	} {
		got, ok := h.Path(tc.src)
		if tc.want == "" {
			if ok {
				t.Errorf("%s: got %s, want nothing", tc.src, got)
			}
			continue
		}
		want, _ := filepath.Abs(tc.want)
		if !ok || got != want {
			t.Errorf("%s: got %q %v, want %q", tc.src, got, ok, want)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/chat@s.whatsapp.net/a%20b.jpg", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "x" {
		t.Fatalf("serve: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/repo/ag1/.git/config", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("serve .git: %d", rec.Code)
	}
}
