package product

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/platform/publicws"
)

const assistantSpeechStreamSegmentRunes = 90

type assistantVoiceStreamer struct {
	ctx       context.Context
	cancel    context.CancelFunc
	handler   *Handler
	userID    string
	runID     string
	locale    string
	callbacks publicws.AssistantCallbacks
	segments  chan string
	done      chan struct{}

	mu     sync.Mutex
	buffer strings.Builder
	closed bool
}

func (h *Handler) newAssistantVoiceStreamer(
	ctx context.Context,
	userID string,
	runID string,
	callbacks publicws.AssistantCallbacks,
) *assistantVoiceStreamer {
	if callbacks.SpeechChunk == nil || callbacks.SpeechError == nil {
		return nil
	}
	streamCtx, cancel := context.WithCancel(ctx)
	streamer := &assistantVoiceStreamer{
		ctx: streamCtx, cancel: cancel, handler: h, userID: userID, runID: runID,
		locale: h.assistantSpeechLocale(ctx, userID), callbacks: callbacks,
		segments: make(chan string, 4), done: make(chan struct{}),
	}
	go streamer.run()
	return streamer
}

func (h *Handler) assistantSpeechLocale(ctx context.Context, userID string) string {
	if h == nil || h.store == nil {
		return "en"
	}
	user, err := h.store.GetUser(ctx, userID)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("assistant speech locale lookup failed", "error_type", fmt.Sprintf("%T", err))
		}
		return "en"
	}
	if user.PreferredLocale != nil && *user.PreferredLocale == "ar" {
		return "ar"
	}
	return "en"
}

func (s *assistantVoiceStreamer) Write(text string) error {
	if s == nil || text == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("assistant speech stream is closed")
	}
	if s.ctx.Err() != nil {
		return nil
	}
	s.buffer.WriteString(text)
	for {
		segment, remainder, ready := takeAssistantSpeechSegment(s.buffer.String(), false)
		if !ready {
			return nil
		}
		s.buffer.Reset()
		s.buffer.WriteString(remainder)
		if err := s.enqueue(segment); err != nil {
			if s.ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func (s *assistantVoiceStreamer) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-s.done
		return
	}
	for strings.TrimSpace(s.buffer.String()) != "" {
		segment, remainder, ready := takeAssistantSpeechSegment(s.buffer.String(), true)
		if !ready {
			break
		}
		s.buffer.Reset()
		s.buffer.WriteString(remainder)
		if err := s.enqueue(segment); err != nil {
			break
		}
	}
	s.closed = true
	close(s.segments)
	s.mu.Unlock()
	<-s.done
	s.cancel()
}

func (s *assistantVoiceStreamer) Abort() {
	if s == nil {
		return
	}
	s.cancel()
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.buffer.Reset()
		close(s.segments)
	}
	s.mu.Unlock()
	<-s.done
}

func (s *assistantVoiceStreamer) enqueue(segment string) error {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return nil
	}
	select {
	case s.segments <- segment:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

func (s *assistantVoiceStreamer) run() {
	defer close(s.done)
	defer s.cancel()
	if s.handler == nil || s.handler.speechOutput == nil || !s.handler.speechOutput.Configured() {
		s.notifyFailure("provider_unavailable", nil)
		return
	}
	for segment := range s.segments {
		if s.ctx.Err() != nil {
			return
		}
		consent, version, err := s.handler.store.VoiceOutputConsent(s.ctx, s.userID)
		if err != nil {
			s.notifyFailure("consent_lookup_failed", err)
			return
		}
		if !consent || version != postgres.VoiceOutputConsentVersion {
			s.notifyFailure("consent_revoked", nil)
			return
		}
		allowed, retryAfter, err := s.handler.allowAssistantVoiceChatSpeech(s.ctx, s.userID)
		if err != nil {
			s.notifyFailure("rate_limit_unavailable", err)
			return
		}
		if !allowed {
			s.notifyFailure("rate_limited_"+retryAfter, nil)
			return
		}
		started := time.Now()
		audio, err := s.handler.speechOutput.Synthesize(s.ctx, segment, s.locale)
		if err != nil {
			if s.ctx.Err() == nil {
				s.notifyFailure("synthesis_failed", err)
			}
			return
		}
		if len(audio) == 0 || len(audio) > 40*1024 {
			s.notifyFailure("audio_chunk_size_invalid", nil)
			return
		}
		if err := s.callbacks.SpeechChunk(s.runID, audio); err != nil {
			return
		}
		if s.handler.logger != nil {
			s.handler.logger.Debug("assistant speech chunk sent",
				"run_id", s.runID,
				"duration_ms", time.Since(started).Milliseconds(),
				"audio_bytes", len(audio),
			)
		}
	}
}

func (s *assistantVoiceStreamer) notifyFailure(reason string, err error) {
	if s == nil || s.ctx.Err() != nil {
		return
	}
	if s.handler != nil && s.handler.logger != nil {
		attributes := []any{"run_id", s.runID, "reason", reason}
		if err != nil {
			attributes = append(attributes, "error_type", fmt.Sprintf("%T", err))
		}
		s.handler.logger.Warn("assistant speech stream failed", attributes...)
	}
	_ = s.callbacks.SpeechError(s.runID)
}

func takeAssistantSpeechSegment(text string, flush bool) (string, string, bool) {
	text = strings.TrimLeft(text, " \t\r\n")
	if text == "" {
		return "", "", false
	}
	runes := []rune(text)
	for index, character := range runes {
		if index+1 < 24 {
			continue
		}
		if !strings.ContainsRune(".!?。！？؟", character) {
			continue
		}
		if index+1 < len(runes) && !unicode.IsSpace(runes[index+1]) {
			continue
		}
		return strings.TrimSpace(string(runes[:index+1])), string(runes[index+1:]), true
	}
	if !flush && len(runes) < assistantSpeechStreamSegmentRunes {
		return "", text, false
	}
	cut := assistantSpeechStreamSegmentRunes
	if cut > len(runes) {
		cut = len(runes)
	}
	for index := cut - 1; index >= 24; index-- {
		if unicode.IsSpace(runes[index]) {
			return strings.TrimSpace(string(runes[:index])), string(runes[index+1:]), true
		}
	}
	if len(runes) > cut {
		return strings.TrimSpace(string(runes[:cut])), string(runes[cut:]), true
	}
	return strings.TrimSpace(text), "", true
}
