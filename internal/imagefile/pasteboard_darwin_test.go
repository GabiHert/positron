package imagefile

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The test uses its own pasteboard, so it never touches the clipboard.
func TestCopyPutsAnImageOnThePasteboard(t *testing.T) {
	png := filepath.Join(t.TempDir(), "dot.png")
	if out, err := exec.Command("/usr/bin/sips", "-s", "format", "png", "-z", "4", "4", "/System/Library/CoreServices/CoreTypes.bundle/Contents/Resources/GenericDocumentIcon.icns", "--out", png).CombinedOutput(); err != nil {
		t.Skipf("no sample image: %v %s", err, out)
	}
	board := fmt.Sprintf("positron-test-%d", time.Now().UnixNano())
	if err := copyTo(png, board); err != nil {
		t.Fatal(err)
	}
	if !boardHasImage(board) {
		t.Fatal("the pasteboard holds no image")
	}
	notImage := filepath.Join(t.TempDir(), "x.jpg")
	_ = os.WriteFile(notImage, []byte("not a picture"), 0o600)
	if err := copyTo(notImage, board+"-2"); err == nil {
		t.Fatal("want an error for a file that is not an image")
	}
}
