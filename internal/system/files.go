package system

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FileHandler serves chat media under /media/ and agents' repo files under /repo/<agent id>/.
type FileHandler struct {
	MediaDir string
	RepoDir  func(agentID string) (string, bool)
}

// inside returns root/rel when it stays inside root and is a file.
func inside(root, rel string) (string, bool) {
	root, _ = filepath.Abs(root)
	p := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if p != root && !strings.HasPrefix(p, root+string(os.PathSeparator)) {
		return "", false
	}
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", false
	}
	return p, true
}

// Path returns the file behind a URL the handler serves: a path such as /media/a/b.jpg, or a
// whole URL whose path is one. Anything else, or a file that is not there, gives false.
func (h FileHandler) Path(src string) (string, bool) {
	path := src
	if u, err := url.Parse(src); err == nil {
		path = u.EscapedPath()
	}
	path, err := url.PathUnescape(path)
	if err != nil {
		return "", false
	}
	switch {
	case strings.HasPrefix(path, "/media/") && h.MediaDir != "":
		return inside(h.MediaDir, strings.TrimPrefix(path, "/media/"))
	case strings.HasPrefix(path, "/repo/") && h.RepoDir != nil:
		id, rel, ok := strings.Cut(strings.TrimPrefix(path, "/repo/"), "/")
		if !ok || rel == ".git" || strings.HasPrefix(rel, ".git/") {
			return "", false
		}
		dir, found := h.RepoDir(id)
		if !found {
			return "", false
		}
		return inside(dir, rel)
	}
	return "", false
}

func (h FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, ok := h.Path(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}
