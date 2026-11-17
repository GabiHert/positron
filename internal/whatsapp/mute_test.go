package whatsapp

import (
	"testing"
	"time"
)

func TestMuteUntil(t *testing.T) {
	at := time.Date(2026, 11, 17, 21, 0, 0, 0, time.UTC)
	cases := []struct {
		end  int64
		want time.Time
	}{
		{0, time.Time{}},
		{-1, MutedForever}, // "always", from app-state and history sync
		{int64(^uint64(0)>>1) * -1, MutedForever},
		{at.UnixMilli(), at}, // app-state: milliseconds
		{at.Unix(), at},      // history sync: seconds
	}
	for _, c := range cases {
		if got := muteUntil(c.end); !got.Equal(c.want) {
			t.Errorf("muteUntil(%d) = %v, want %v", c.end, got, c.want)
		}
	}
	// History sync hands "always" over as an unsigned -1.
	unsigned := ^uint64(0)
	if got := muteUntil(int64(unsigned)); !got.Equal(MutedForever) {
		t.Errorf("unsigned -1 = %v", got)
	}
}
