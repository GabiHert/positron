// Package imagefile does what the image viewer's buttons do with a picture Positron serves:
// save a copy, put it on the clipboard, or open it in Preview.
package imagefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Files works on the files behind image URLs.
type Files struct {
	// Resolve turns a URL the window shows (/media/…, /repo/…) into the file on disk.
	Resolve func(src string) (string, bool)
	// SaveDialog asks where to save; "" means the person cancelled.
	SaveDialog func(defaultName string) (string, error)
	// Copy puts the image at path on the clipboard.
	Copy func(path string) error
	// Open runs /usr/bin/open with args; nil runs the real one.
	Open func(args ...string) error
}

var errGone = errors.New("this image is no longer on this Mac")

func (f Files) path(src string) (string, error) {
	if f.Resolve == nil {
		return "", errGone
	}
	p, ok := f.Resolve(src)
	if !ok {
		return "", errGone
	}
	return p, nil
}

// SuggestedName is the file name the save dialog starts with: name when given, with the file's
// extension, else the file's own name.
func SuggestedName(name, path string) string {
	ext := filepath.Ext(path)
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '/' || r == ':' || r < 0x20 {
			return '-'
		}
		return r
	}, name))
	if name == "" {
		return filepath.Base(path)
	}
	if ext != "" && !strings.EqualFold(filepath.Ext(name), ext) {
		name += ext
	}
	return name
}

// Save asks where to save a copy of the image and writes it there. It returns the new file's
// path, or "" when the person cancelled.
func (f Files) Save(src, name string) (string, error) {
	p, err := f.path(src)
	if err != nil {
		return "", err
	}
	if f.SaveDialog == nil {
		return "", errors.New("saving is not available")
	}
	dest, err := f.SaveDialog(SuggestedName(name, p))
	if err != nil || dest == "" {
		return "", err
	}
	if same(p, dest) {
		return dest, nil
	}
	if err := copyFile(p, dest); err != nil {
		return "", fmt.Errorf("could not save the image: %w", err)
	}
	return dest, nil
}

// CopyImage puts the image on the clipboard, ready to paste into another app.
func (f Files) CopyImage(src string) error {
	p, err := f.path(src)
	if err != nil {
		return err
	}
	if f.Copy == nil {
		return errors.New("copying images is not available")
	}
	return f.Copy(p)
}

// OpenInPreview opens the image in Preview.
func (f Files) OpenInPreview(src string) error {
	p, err := f.path(src)
	if err != nil {
		return err
	}
	open := f.Open
	if open == nil {
		open = func(args ...string) error { return exec.Command("/usr/bin/open", args...).Run() }
	}
	return open("-a", "Preview", p)
}

func same(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

// copyFile writes src to dest through a temporary file, so a failed copy leaves dest as it was.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".positron-save-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}
