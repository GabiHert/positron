package backend

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/domain"
)

// Mute lengths, the same choices WhatsApp offers.
const (
	Mute8Hours = "8h"
	Mute1Week  = "1w"
	MuteAlways = "always"
	MuteOff    = "off"
)

// muteEnd is when a mute of the given length that starts at now ends: domain.Forever for
// always, zero to unmute.
func muteEnd(length string, now time.Time) (time.Time, error) {
	switch strings.ToLower(strings.TrimSpace(length)) {
	case Mute8Hours, "8 hours":
		return now.Add(8 * time.Hour), nil
	case Mute1Week, "week", "1 week", "7d":
		return now.AddDate(0, 0, 7), nil
	case MuteAlways, "forever":
		return domain.Forever, nil
	case MuteOff, "unmute", "none":
		return time.Time{}, nil
	}
	return time.Time{}, fmt.Errorf("mute for %q? Choose 8h, 1w, always or off", length)
}

// MuteChat mutes a chat on WhatsApp (8h, 1w or always) or unmutes it (off). The phone and
// WhatsApp's other devices show the same. It returns when the mute ends, in Unix ms
// (0 = not muted).
func (b *Backend) MuteChat(jid, length string) (int64, error) {
	until, err := muteEnd(length, time.Now())
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := b.d.Engine.MuteChat(ctx, jid, until)
	if err != nil {
		return 0, err
	}
	if c.Muted(time.Now()) {
		b.dropNotify(jid) // a waiting notification for it doesn't go out
		return ms(c.MutedUntil), nil
	}
	b.emit(app.EventChats, nil)
	return 0, nil
}
