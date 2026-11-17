package backend

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/domain"
)

// MessageFile returns a message and the file behind its media on this Mac, downloading it from
// WhatsApp first when Positron does not have it yet. View-once media is refused: it is meant to be
// seen once, in Positron's window.
func (b *Backend) MessageFile(ctx context.Context, chat, id string) (app.ChatMessageDTO, string, error) {
	m, err := b.d.Store.FindMessage(chat, id)
	if err != nil {
		return app.ChatMessageDTO{}, "", errors.New("that message is gone")
	}
	if m.ViewOnce {
		return b.chatMessageDTO(m), "", errors.New("this is a view-once message: it can only be seen once, in Positron's window")
	}
	if !isMedia(m.Kind) {
		return b.chatMessageDTO(m), "", errors.New("this message has no photo, video, audio or file")
	}
	saved, p, err := b.mediaFile(ctx, chat, id)
	if err != nil {
		return b.chatMessageDTO(m), "", err
	}
	return b.chatMessageDTO(saved), p, nil
}

// TranscribeMessage returns a voice note's transcript, transcribing it now when it has none and
// keeping the result. lang is used when the chat has no agent; "" picks an installed language.
func (b *Backend) TranscribeMessage(ctx context.Context, chat, id, lang string) (string, error) {
	m, p, err := b.mediaFile(ctx, chat, id)
	if err != nil {
		return "", err
	}
	if m.Kind != domain.KindVoice && m.Kind != domain.KindAudio {
		return "", errors.New("only voice notes and audio have transcripts")
	}
	if m.ViewOnce {
		return "", errors.New("view-once voice notes are not transcribed")
	}
	if m.Transcript != "" {
		return m.Transcript, nil
	}
	text, err := b.d.Engine.TranscribeFile(ctx, p, b.transcriptLanguage(chat, lang))
	if err != nil {
		return "", err
	}
	if saved, ok, _ := b.d.Store.UpdateMessage(m.ChatJID, m.ID, func(s *domain.Message) bool {
		s.Transcript = text
		return true
	}); ok {
		b.OnMessage(saved, "update")
	}
	return text, nil
}

// transcriptLanguage is the chat's agent's language, else the one asked for, else the first one
// whose model is installed.
func (b *Backend) transcriptLanguage(chat, asked string) domain.Language {
	for _, a := range b.d.Store.Agents() {
		if slices.Contains(a.Chats, chat) && a.Language != "" {
			return a.Language
		}
	}
	for _, l := range domain.Languages {
		if strings.EqualFold(string(l), strings.TrimSpace(asked)) {
			return l
		}
	}
	if b.d.Models != nil {
		for _, l := range domain.Languages {
			if b.d.Models.Status(l).Ready {
				return l
			}
		}
	}
	return domain.Languages[0]
}
