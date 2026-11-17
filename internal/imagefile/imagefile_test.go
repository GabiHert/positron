package imagefile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func setup(t *testing.T) (Files, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "3EB0A1.jpg")
	if err := os.WriteFile(p, []byte("jpeg bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Files{Resolve: func(src string) (string, bool) { return p, src == "/media/c/3EB0A1.jpg" }}, p
}

func TestSuggestedName(t *testing.T) {
	for _, tc := range []struct{ name, path, want string }{
		{"", "/m/3EB0A1.jpg", "3EB0A1.jpg"},
		{"Photo from Ana", "/m/3EB0A1.jpg", "Photo from Ana.jpg"},
		{"beach.JPG", "/m/3EB0A1.jpg", "beach.JPG"},
		{"a/b: c", "/m/x.png", "a-b- c.png"},
		{"  ", "/m/x.webp", "x.webp"},
	} {
		if got := SuggestedName(tc.name, tc.path); got != tc.want {
			t.Errorf("SuggestedName(%q, %q) = %q, want %q", tc.name, tc.path, got, tc.want)
		}
	}
}

func TestSaveCopiesTheFileWhereThePersonChose(t *testing.T) {
	f, _ := setup(t)
	dest := filepath.Join(t.TempDir(), "saved.jpg")
	var asked string
	f.SaveDialog = func(name string) (string, error) { asked = name; return dest, nil }
	got, err := f.Save("/media/c/3EB0A1.jpg", "Photo from Ana")
	if err != nil || got != dest || asked != "Photo from Ana.jpg" {
		t.Fatalf("got %q %v, asked %q", got, err, asked)
	}
	if b, _ := os.ReadFile(dest); string(b) != "jpeg bytes" {
		t.Fatalf("saved %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".positron-save-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

func TestSaveCancelledWritesNothing(t *testing.T) {
	f, _ := setup(t)
	f.SaveDialog = func(string) (string, error) { return "", nil }
	if got, err := f.Save("/media/c/3EB0A1.jpg", ""); got != "" || err != nil {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestSaveOntoItselfKeepsTheFile(t *testing.T) {
	f, p := setup(t)
	f.SaveDialog = func(string) (string, error) { return p, nil }
	if _, err := f.Save("/media/c/3EB0A1.jpg", ""); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "jpeg bytes" {
		t.Fatalf("file now %q", b)
	}
}

func TestUnknownImagesAreRefused(t *testing.T) {
	f, _ := setup(t)
	f.SaveDialog = func(string) (string, error) { t.Fatal("dialog opened"); return "", nil }
	f.Copy = func(string) error { t.Fatal("copied"); return nil }
	f.Open = func(...string) error { t.Fatal("opened"); return nil }
	if _, err := f.Save("/media/other.jpg", ""); err == nil {
		t.Error("save: want an error")
	}
	if err := f.CopyImage("/etc/hosts"); err == nil {
		t.Error("copy: want an error")
	}
	if err := f.OpenInPreview("/repo/x/y.png"); err == nil {
		t.Error("open: want an error")
	}
}

func TestCopyAndPreviewGetTheFile(t *testing.T) {
	f, p := setup(t)
	var copied string
	var opened []string
	f.Copy = func(path string) error { copied = path; return nil }
	f.Open = func(args ...string) error { opened = args; return nil }
	if err := f.CopyImage("/media/c/3EB0A1.jpg"); err != nil || copied != p {
		t.Fatalf("copy %q %v", copied, err)
	}
	if err := f.OpenInPreview("/media/c/3EB0A1.jpg"); err != nil || !reflect.DeepEqual(opened, []string{"-a", "Preview", p}) {
		t.Fatalf("open %v %v", opened, err)
	}
}
