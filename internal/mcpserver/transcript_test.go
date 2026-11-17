package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GabiHert/positron/internal/app"
)

// Message reads carry a voice note's stored transcript.
func TestMessagesCarryTheirTranscript(t *testing.T) {
	out, _ := json.Marshal(toMsg(app.ChatMessageDTO{ID: "v1", Kind: "voice", Transcript: "Chego às sete"}))
	if !strings.Contains(string(out), `"transcript":"Chego às sete"`) {
		t.Fatalf("%s", out)
	}
}
