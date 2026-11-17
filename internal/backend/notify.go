package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/domain"
)

// ChatNotification is one chat's notification. A newer one for the same chat replaces it.
type ChatNotification struct {
	Chat     string
	Title    string
	Subtitle string
	Body     string
	Sound    bool
	Image    string // the chat's picture on disk, "" for none
}

const (
	// notifyMaxAge: older messages (a backlog after sleep) don't pop up.
	notifyMaxAge = time.Hour
	// notifyGather is how long a chat's notification waits for more messages, so a burst shows
	// once as "3 new messages" instead of three banners.
	notifyGather = 1200 * time.Millisecond
)

// notifier holds each chat's newest message until its notification goes out.
type notifier struct {
	mu      sync.Mutex
	pending map[string]domain.Message
	timers  map[string]*time.Timer
	gather  time.Duration
}

func (b *Backend) nt() *notifier {
	b.notifyOnce.Do(func() {
		b.notifyState = &notifier{pending: map[string]domain.Message{}, timers: map[string]*time.Timer{}, gather: notifyGather}
	})
	return b.notifyState
}

// shouldNotify decides whether m may show a notification, before looking at the chat's state.
func shouldNotifyMessage(m domain.Message, now time.Time) bool {
	return !m.FromMe && !m.FromAgent && !m.History && !m.Offline && !m.Revoked && now.Sub(m.At) <= notifyMaxAge
}

// chatAllows applies the owner's settings and the chat's state. open is true when the chat is
// on screen in a focused window.
func chatAllows(s domain.Settings, c domain.Chat, m domain.Message, open bool, now time.Time) bool {
	if !s.NotifyMessages || open || c.Muted(now) || c.NotifyOff || c.Archived || c.Unread == 0 {
		return false
	}
	if c.IsGroup {
		switch s.NotifyGroups {
		case domain.GroupsOff:
			return false
		case domain.GroupsMentions:
			return m.MentionsMe
		}
	}
	return true
}

// notificationText is what the notification says: "Ana" or "Ana @ Family" over the newest
// message, with "3 new messages" when more are unread. Without previews it hides the text.
func notificationText(s domain.Settings, c domain.Chat, m domain.Message) (title, subtitle, body string) {
	title = firstNonEmpty(c.Name, m.SenderName, "WhatsApp")
	if c.IsGroup && s.NotifyPreview {
		if who := firstNonEmpty(m.SenderName, phoneOf(m.SenderJID)); who != "" {
			title = who + " @ " + title
		}
	}
	more := ""
	if c.Unread > 1 {
		more = fmt.Sprintf("%d new messages", c.Unread)
	}
	if !s.NotifyPreview {
		return title, "", firstNonEmpty(more, "New message")
	}
	body = firstNonEmpty(snippet(m), "New message")
	return title, more, body
}

func phoneOf(jid string) string {
	for i, r := range jid {
		if r == '@' {
			return "+" + jid[:i]
		}
	}
	return ""
}

// queueNotify holds m for a moment and sends the chat's notification when no newer message came.
func (b *Backend) queueNotify(m domain.Message) {
	if b.d.NotifyChat == nil || !shouldNotifyMessage(m, time.Now()) {
		return
	}
	n := b.nt()
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending[m.ChatJID] = m
	if t := n.timers[m.ChatJID]; t != nil {
		t.Stop()
	}
	chat := m.ChatJID
	n.timers[chat] = time.AfterFunc(n.gather, func() { b.flushNotify(chat) })
}

// flushNotify sends chat's held notification, checking again: the chat may have been read or
// opened meanwhile.
func (b *Backend) flushNotify(chat string) {
	n := b.nt()
	n.mu.Lock()
	m, ok := n.pending[chat]
	delete(n.pending, chat)
	delete(n.timers, chat)
	n.mu.Unlock()
	if !ok {
		return
	}
	c, err := b.d.Store.Chat(chat)
	if err != nil {
		return
	}
	s := b.d.Store.Settings()
	if !chatAllows(s, c, m, b.chatOnScreen(chat), time.Now()) {
		return
	}
	title, subtitle, body := notificationText(s, c, m)
	out := ChatNotification{Chat: chat, Title: title, Subtitle: subtitle, Body: body, Sound: s.NotifySound}
	if s.NotifyPreview {
		out.Image = b.avatarFile(chat)
	}
	b.d.NotifyChat(out)
}

// chatOnScreen is true when chat is open in a window that has the focus.
func (b *Backend) chatOnScreen(chat string) bool {
	ib := b.ib()
	ib.mu.Lock()
	active := ib.active
	ib.mu.Unlock()
	return active == chat && b.d.Focused != nil && b.d.Focused()
}

// avatarFile is the chat's cached picture, never fetched for a notification.
func (b *Backend) avatarFile(jid string) string {
	e, ok := b.avatarEntry(jid)
	if !ok || e.File == "" || b.d.Media.Dir == "" {
		return ""
	}
	p := filepath.Join(b.d.Media.Dir, filepath.FromSlash(e.File))
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// dropNotify forgets a chat's held notification and takes its delivered one away.
func (b *Backend) dropNotify(chat string) {
	n := b.nt()
	n.mu.Lock()
	if t := n.timers[chat]; t != nil {
		t.Stop()
	}
	delete(n.timers, chat)
	delete(n.pending, chat)
	n.mu.Unlock()
	if b.d.ClearNotification != nil {
		b.d.ClearNotification(notificationID(chat))
	}
}

func notificationID(chat string) string { return "chat:" + chat }

// SetChatNotifications mutes or unmutes a chat in Positron only (WhatsApp's mute is separate).
func (b *Backend) SetChatNotifications(jid string, on bool) error {
	if _, err := b.d.Store.UpdateChat(jid, func(c *domain.Chat) { c.NotifyOff = !on }); err != nil {
		return err
	}
	if !on {
		b.dropNotify(jid)
	}
	b.OnUnread(jid) // the Dock badge leaves muted chats out
	b.emit(app.EventChats, nil)
	return nil
}

// groupsSetting keeps NotifyGroups to its known values; anything else means all groups.
func groupsSetting(v string) string {
	switch v {
	case domain.GroupsMentions, domain.GroupsOff:
		return v
	}
	return domain.GroupsAll
}

// OpenNotificationSettings opens Positron's page in System Settings → Notifications.
func (b *Backend) OpenNotificationSettings() error {
	return exec.Command("/usr/bin/open", "x-apple.systempreferences:com.apple.Notifications-Settings.extension?id=tech.bhlabs.positron").Run()
}
