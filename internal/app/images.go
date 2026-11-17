package app

import (
	"context"
	"errors"

	"github.com/GabiHert/positron/internal/imagefile"
)

// ImageFiles is what the image viewer's buttons need; the dialog gets the Wails context.
type ImageFiles struct {
	Resolve    func(src string) (string, bool)
	SaveDialog func(ctx context.Context, defaultName string) (string, error)
	Copy       func(path string) error
}

// SetImageFiles wires the image viewer. It is a function, not a method, so it is not bound to the UI.
func SetImageFiles(a *App, f ImageFiles) { a.images = f }

func (a *App) imageFiles() (imagefile.Files, error) {
	if a.images.Resolve == nil {
		return imagefile.Files{}, errors.New("images are not available")
	}
	f := imagefile.Files{Resolve: a.images.Resolve, Copy: a.images.Copy}
	if a.images.SaveDialog != nil {
		f.SaveDialog = func(name string) (string, error) { return a.images.SaveDialog(a.ctx, name) }
	}
	return f, nil
}

// SaveImage asks where to save a copy of the image at src (a /media/ or /repo/ URL) and saves
// it. name is the file name to suggest. It returns the saved path, or "" when cancelled.
func (a *App) SaveImage(src, name string) (string, error) {
	f, err := a.imageFiles()
	if err != nil {
		return "", err
	}
	return f.Save(src, name)
}

// CopyImage puts the image at src on the clipboard.
func (a *App) CopyImage(src string) error {
	f, err := a.imageFiles()
	if err != nil {
		return err
	}
	return f.CopyImage(src)
}

// OpenImageInPreview opens the image at src in Preview.
func (a *App) OpenImageInPreview(src string) error {
	f, err := a.imageFiles()
	if err != nil {
		return err
	}
	return f.OpenInPreview(src)
}
