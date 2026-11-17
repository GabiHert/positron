package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Downloads and exports write only inside the person's home folder, where they asked.

type downloadIn struct {
	Chat       string `json:"chat" jsonschema:"the chat: name, phone number or JID"`
	MessageID  string `json:"message_id" jsonschema:"the message's id, from read_whatsapp_chat or search_whatsapp_messages"`
	Folder     string `json:"folder,omitempty" jsonschema:"folder to save into, inside the home folder (~ works); default ~/Downloads/Positron"`
	Transcript bool   `json:"transcript,omitempty" jsonschema:"for a voice note or audio: also return its transcript, transcribing it now if needed"`
	Language   string `json:"language,omitempty" jsonschema:"transcript language when the chat has no agent: pt-BR or en"`
}

type exportIn struct {
	Chat         string `json:"chat" jsonschema:"the chat: name, phone number or JID"`
	Since        string `json:"since,omitempty" jsonschema:"first day or time to include, like 2026-11-01 or 2026-11-01 14:30"`
	Until        string `json:"until,omitempty" jsonschema:"last day or time to include (a day includes all of it)"`
	Last         int    `json:"last,omitempty" jsonschema:"the last N messages; used when since and until are left out (default 200)"`
	Format       string `json:"format,omitempty" jsonschema:"markdown (default) or json"`
	Folder       string `json:"folder,omitempty" jsonschema:"folder to write into, inside the home folder (~ works); default ~/Downloads/Positron"`
	IncludeMedia bool   `json:"include_media,omitempty" jsonschema:"also save the chat's photos, videos, audio and files in a folder next to the export"`
}

// maxExport bounds one export.
const maxExport = 20000

// ---- paths ----

func (s *Server) userHome() (string, error) {
	h := s.opts.UserHome
	if h == "" {
		var err error
		if h, err = os.UserHomeDir(); err != nil {
			return "", err
		}
	}
	return realPath(h)
}

// realPath resolves symlinks in p, also when its last parts do not exist yet.
func realPath(p string) (string, error) {
	p = filepath.Clean(p)
	rest := ""
	for {
		r, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(r, rest), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest), nil
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// folder checks the folder the caller chose (or the default) is inside the home folder and makes it.
func (s *Server) folder(dir string) (string, error) {
	home, err := s.userHome()
	if err != nil {
		return "", err
	}
	dir = strings.TrimSpace(dir)
	switch {
	case dir == "":
		dir = filepath.Join(home, "Downloads", "Positron")
	case dir == "~":
		dir = home
	case strings.HasPrefix(dir, "~/"):
		dir = filepath.Join(home, dir[2:])
	case !filepath.IsAbs(dir):
		return "", fmt.Errorf("give the folder as a full path, like ~/Downloads, not %q", dir)
	}
	real, err := realPath(dir)
	if err != nil {
		return "", err
	}
	if real != home && !strings.HasPrefix(real, home+string(os.PathSeparator)) {
		return "", fmt.Errorf("%s is outside your home folder (%s); Positron only saves inside it", dir, home)
	}
	if err := os.MkdirAll(real, 0o755); err != nil {
		return "", fmt.Errorf("could not make the folder: %w", err)
	}
	return real, nil
}

// cleanName makes s safe as a file name.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == ':' || r == '\\' || r < 0x20 || r == 0x7f:
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	s = strings.TrimLeft(s, ".")
	if len(s) > 180 {
		s = s[:180]
	}
	return strings.TrimSpace(s)
}

// create makes a new file in dir named name, adding " (2)", " (3)"… instead of replacing a file.
func create(dir, name string) (*os.File, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		n := name
		if i > 1 {
			n = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		f, err := os.OpenFile(filepath.Join(dir, n), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, errors.New("too many files with that name")
}

func copyInto(dir, name, src string) (string, int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	out, err := create(dir, name)
	if err != nil {
		return "", 0, err
	}
	n, err := io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(out.Name())
		return "", 0, err
	}
	return out.Name(), n, nil
}

var kindNames = map[string]string{
	"image": "photo", "video": "video", "voice": "voice note", "audio": "audio",
	"document": "document", "sticker": "sticker",
}

// mediaName is the file's own name for documents, else like "WhatsApp photo 2026-11-16 at 14.03.27.jpg".
func mediaName(m app.ChatMessageDTO, path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if m.Media != nil {
		if n := cleanName(m.Media.FileName); n != "" {
			if filepath.Ext(n) == "" {
				n += ext
			}
			return n
		}
	}
	if ext == "" && m.Media != nil {
		if exts, _ := mime.ExtensionsByType(m.Media.Mime); len(exts) > 0 {
			ext = exts[0]
		}
	}
	kind := kindNames[m.Kind]
	if kind == "" {
		kind = "file"
	}
	t := time.UnixMilli(m.At).Local()
	return fmt.Sprintf("WhatsApp %s %s at %s%s", kind, t.Format("2006-01-02"), t.Format("15.04.05"), ext)
}

func mimeOf(m app.ChatMessageDTO, path string) string {
	if m.Media != nil && m.Media.Mime != "" {
		return strings.TrimSpace(strings.SplitN(m.Media.Mime, ";", 2)[0])
	}
	if t := mime.TypeByExtension(filepath.Ext(path)); t != "" {
		return strings.SplitN(t, ";", 2)[0]
	}
	return "application/octet-stream"
}

// ---- tools ----

type savedOut struct {
	Path            string `json:"path"`
	Mime            string `json:"mime"`
	Size            int64  `json:"size"`
	Kind            string `json:"kind"`
	From            string `json:"from"`
	When            string `json:"when"`
	Caption         string `json:"caption,omitempty"`
	Transcript      string `json:"transcript,omitempty"`
	TranscriptError string `json:"transcriptError,omitempty"`
}

func (s *Server) mediaTools(srv *mcp.Server, findChat func(string) (app.ChatDTO, error)) {
	b := s.b

	add(srv, write("download_whatsapp_media", "Save a WhatsApp message's photo, video, audio, voice note, document or sticker to a folder in the owner's home folder (default ~/Downloads/Positron), fetching it from WhatsApp if Positron doesn't have it yet. Returns the path, MIME type and size. Never replaces a file. Media WhatsApp has deleted from its servers can't be fetched; view-once media is refused."),
		func(ctx context.Context, in downloadIn) (any, error) {
			c, err := findChat(in.Chat)
			if err != nil {
				return nil, err
			}
			dir, err := s.folder(in.Folder)
			if err != nil {
				return nil, err
			}
			m, src, err := b.MessageFile(ctx, c.JID, in.MessageID)
			if err != nil {
				return nil, err
			}
			p, n, err := copyInto(dir, mediaName(m, src), src)
			if err != nil {
				return nil, fmt.Errorf("could not save the file: %w", err)
			}
			mo := toMsg(m)
			out := savedOut{Path: p, Mime: mimeOf(m, src), Size: n, Kind: m.Kind, From: mo.From, When: mo.When, Caption: m.Text, Transcript: m.Transcript}
			if in.Transcript && out.Transcript == "" {
				if m.Kind != "voice" && m.Kind != "audio" {
					out.TranscriptError = "only voice notes and audio have transcripts"
				} else if t, err := b.TranscribeMessage(ctx, c.JID, m.ID, in.Language); err != nil {
					out.TranscriptError = err.Error()
				} else {
					out.Transcript = t
				}
			}
			return out, nil
		})

	add(srv, write("export_whatsapp_chat", "Export a WhatsApp chat's stored messages to a Markdown or JSON file in the owner's home folder (default ~/Downloads/Positron): sender, time, text, voice transcripts, files and what each replies to. Pick a range with since/until, or the last N messages. include_media also saves the media in a folder next to it. Returns the path. Only what Positron has stored is exported; read_whatsapp_chat shows how far back that goes."),
		func(ctx context.Context, in exportIn) (any, error) {
			c, err := findChat(in.Chat)
			if err != nil {
				return nil, err
			}
			format := strings.ToLower(strings.TrimSpace(in.Format))
			switch format {
			case "", "md", "markdown":
				format = "markdown"
			case "json":
			default:
				return nil, errors.New("format is markdown or json")
			}
			since, err := parseWhen(in.Since, false)
			if err != nil {
				return nil, err
			}
			until, err := parseWhen(in.Until, true)
			if err != nil {
				return nil, err
			}
			if !since.IsZero() && !until.IsZero() && until.Before(since) {
				return nil, errors.New("until is before since")
			}
			last := in.Last
			if since.IsZero() && until.IsZero() && last <= 0 {
				last = 200
			}
			if last > maxExport {
				return nil, fmt.Errorf("at most %d messages per export", maxExport)
			}
			dir, err := s.folder(in.Folder)
			if err != nil {
				return nil, err
			}
			msgs, truncated, err := collect(b, c.JID, since, until, last)
			if err != nil {
				return nil, err
			}
			if len(msgs) == 0 {
				return nil, errors.New("Positron has no messages from this chat in that range")
			}
			span := day(msgs[0].At)
			if end := day(msgs[len(msgs)-1].At); end != span {
				span += " to " + end
			}
			name := cleanName(fmt.Sprintf("WhatsApp %s %s", orJID(c), span))
			ext := map[string]string{"markdown": ".md", "json": ".json"}[format]
			f, err := create(dir, name+ext)
			if err != nil {
				return nil, fmt.Errorf("could not write the export: %w", err)
			}
			base := strings.TrimSuffix(filepath.Base(f.Name()), ext)

			rows := make([]exportRow, len(msgs))
			res := exportResult{Messages: len(msgs), Truncated: truncated}
			mediaDir := ""
			for i, m := range msgs {
				rows[i] = row(m)
				if !in.IncludeMedia || m.Media == nil || m.Revoked {
					continue
				}
				if m.ViewOnce {
					rows[i].MediaError = "view once: only in Positron's window"
					res.MediaMissing++
					continue
				}
				_, src, err := b.MessageFile(ctx, c.JID, m.ID)
				if err == nil && mediaDir == "" {
					mediaDir = filepath.Join(dir, base+" media")
					err = os.MkdirAll(mediaDir, 0o755)
				}
				var p string
				if err == nil {
					p, _, err = copyInto(mediaDir, mediaName(m, src), src)
				}
				if err != nil {
					rows[i].MediaError = err.Error()
					res.MediaMissing++
					continue
				}
				rows[i].Saved = filepath.Join(filepath.Base(mediaDir), filepath.Base(p))
				res.MediaSaved++
			}
			var werr error
			if format == "json" {
				enc := json.NewEncoder(f)
				enc.SetIndent("", " ")
				werr = enc.Encode(struct {
					Chat     string      `json:"chat"`
					JID      string      `json:"jid"`
					Exported string      `json:"exported"`
					Messages []exportRow `json:"messages"`
				}{orJID(c), c.JID, time.Now().Format(time.RFC3339), rows})
			} else {
				_, werr = io.WriteString(f, markdown(orJID(c), rows))
			}
			if cerr := f.Close(); werr == nil {
				werr = cerr
			}
			if werr != nil {
				_ = os.Remove(f.Name())
				return nil, fmt.Errorf("could not write the export: %w", werr)
			}
			res.Path = f.Name()
			if mediaDir != "" {
				res.MediaFolder = mediaDir
			}
			res.From, res.To = when(msgs[0].At), when(msgs[len(msgs)-1].At)
			return res, nil
		})
}

type exportResult struct {
	Path         string `json:"path"`
	Messages     int    `json:"messages"`
	From         string `json:"from"`
	To           string `json:"to"`
	Truncated    bool   `json:"truncated,omitempty"`
	MediaFolder  string `json:"mediaFolder,omitempty"`
	MediaSaved   int    `json:"mediaSaved,omitempty"`
	MediaMissing int    `json:"mediaMissing,omitempty"`
}

func day(ms int64) string { return time.UnixMilli(ms).Local().Format("2006-01-02") }

// parseWhen reads a day or a day and time in local time. A bare day as an end means its last moment.
func parseWhen(s string, end bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		if end {
			t = t.AddDate(0, 0, 1).Add(-time.Millisecond)
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%q is not a date: use 2026-11-01 or 2026-11-01 14:30", s)
}

// collect pages back through the stored messages and returns the range oldest first.
func collect(b Backend, jid string, since, until time.Time, last int) ([]app.ChatMessageDTO, bool, error) {
	var out []app.ChatMessageDTO
	cursor := ""
	limit := maxExport
	if last > 0 {
		limit = last
	}
	truncated := false
	for {
		page, err := b.ListMessages(jid, cursor, 200)
		if err != nil {
			return nil, false, err
		}
		done := false
		for _, m := range page.Messages { // newest first
			t := time.UnixMilli(m.At)
			if !until.IsZero() && t.After(until) {
				continue
			}
			if !since.IsZero() && t.Before(since) {
				done = true
				break
			}
			if len(out) == limit {
				truncated = last <= 0 // a "last N" export is never cut short, only a range
				done = true
				break
			}
			out = append(out, m)
		}
		if done || page.Cursor == "" {
			break
		}
		cursor = page.Cursor
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, truncated, nil
}

type exportRow struct {
	ID         string     `json:"id"`
	When       string     `json:"when"`
	At         string     `json:"at"`
	From       string     `json:"from"`
	Kind       string     `json:"kind"`
	Text       string     `json:"text,omitempty"`
	Transcript string     `json:"transcript,omitempty"`
	Media      *mediaInfo `json:"media,omitempty"`
	Saved      string     `json:"saved,omitempty"` // the media file, relative to the export
	MediaError string     `json:"mediaError,omitempty"`
	ReplyTo    string     `json:"replyTo,omitempty"`
	Reactions  []string   `json:"reactions,omitempty"`
	Edited     bool       `json:"edited,omitempty"`
	Deleted    bool       `json:"deleted,omitempty"`
}

func row(m app.ChatMessageDTO) exportRow {
	o := toMsg(m)
	return exportRow{ID: m.ID, When: o.When, At: time.UnixMilli(m.At).Local().Format(time.RFC3339), From: o.From, Kind: m.Kind,
		Text: m.Text, Transcript: m.Transcript, Media: o.Media, ReplyTo: o.ReplyTo, Reactions: o.Reactions, Edited: m.Edited, Deleted: m.Revoked}
}

// markdown writes one line per message, grouped by day, the way a person reads a chat.
func markdown(chat string, rows []exportRow) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# WhatsApp: %s\n\n", chat)
	fmt.Fprintf(&sb, "%d messages, %s to %s. Exported by Positron on %s.\n", len(rows), rows[0].When, rows[len(rows)-1].When, time.Now().Format("2006-01-02 15:04"))
	lastDay := ""
	for _, r := range rows {
		if d := r.When[:10]; d != lastDay {
			fmt.Fprintf(&sb, "\n## %s\n\n", d)
			lastDay = d
		}
		fmt.Fprintf(&sb, "**%s** %s", r.When[11:], mdEscape(r.From))
		if r.Deleted {
			sb.WriteString(": _deleted_\n\n")
			continue
		}
		sb.WriteString(":")
		if r.Media != nil {
			label := r.Media.Label()
			if r.Saved != "" {
				fmt.Fprintf(&sb, " [%s](<%s>)", label, r.Saved)
			} else {
				fmt.Fprintf(&sb, " [%s]", label)
			}
			if r.MediaError != "" {
				fmt.Fprintf(&sb, " (not saved: %s)", r.MediaError)
			}
		} else if r.Kind != "text" && r.Kind != "" {
			fmt.Fprintf(&sb, " [%s]", r.Kind)
		}
		if r.Text != "" {
			sb.WriteString(" " + strings.ReplaceAll(strings.TrimSpace(r.Text), "\n", "  \n"))
		}
		if r.Transcript != "" {
			fmt.Fprintf(&sb, "  \n> transcript: %s", strings.ReplaceAll(r.Transcript, "\n", " "))
		}
		if r.ReplyTo != "" {
			fmt.Fprintf(&sb, "  \n> replying to %s", strings.ReplaceAll(r.ReplyTo, "\n", " "))
		}
		if r.Edited {
			sb.WriteString(" _(edited)_")
		}
		if len(r.Reactions) > 0 {
			fmt.Fprintf(&sb, "  \n> reactions: %s", strings.Join(r.Reactions, "; "))
		}
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func mdEscape(s string) string {
	return strings.NewReplacer("*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`).Replace(s)
}
