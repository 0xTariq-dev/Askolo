package product

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	assistantMaxReplyBytes      = 4096
	assistantMaxStreamBodyBytes = 64 * 1024
	assistantMaxStreamLineBytes = 32 * 1024
)

type assistantPlannerStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

func readAssistantPlannerStream(
	reader io.Reader,
	tools *assistantToolRegistry,
	onDelta func(string) error,
) (assistantModelPlan, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, assistantMaxStreamBodyBytes+1))
	scanner.Buffer(make([]byte, 4096), assistantMaxStreamLineBytes)
	content := &strings.Builder{}
	messageStream := assistantPlanMessageStream{onDelta: onDelta}
	var plan assistantModelPlan
	done := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			done = true
			break
		}
		if content.Len()+len(data) > assistantMaxStreamBodyBytes {
			return plan, errors.New("assistant provider stream exceeded the size limit")
		}
		var chunk assistantPlannerStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return plan, errors.New("assistant provider stream was invalid")
		}
		if chunk.ID != "" {
			plan.ProviderRequestID = chunk.ID
		}
		if chunk.Model != "" {
			plan.ProviderModel = chunk.Model
		}
		if chunk.Usage != nil {
			plan.InputTokens = chunk.Usage.PromptTokens
			plan.OutputTokens = chunk.Usage.CompletionTokens
			plan.UsageValid = chunk.Usage.PromptTokens > 0 && chunk.Usage.CompletionTokens > 0
		}
		if len(chunk.Choices) != 1 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		if content.Len()+len(delta) > assistantMaxOutputBytes {
			return plan, errors.New("assistant provider response exceeded the size limit")
		}
		content.WriteString(delta)
		if err := messageStream.Append(delta); err != nil {
			return plan, err
		}
	}
	if err := scanner.Err(); err != nil {
		return plan, errors.New("assistant provider stream could not be read")
	}
	if !done && content.Len() == 0 {
		return plan, errors.New("assistant provider stream was empty")
	}
	if content.Len() == 0 {
		return plan, errors.New("assistant provider stream did not contain a plan")
	}
	decoded, decodeErr := decodeAssistantModelPlan(content.String(), tools)
	decoded.ProviderModel = plan.ProviderModel
	decoded.ProviderRequestID = plan.ProviderRequestID
	decoded.InputTokens = plan.InputTokens
	decoded.OutputTokens = plan.OutputTokens
	decoded.UsageValid = plan.UsageValid
	if decoded.ProviderModel != assistantPlannerModel {
		return decoded, errors.New("assistant provider model mismatch")
	}
	if decodeErr != nil {
		return decoded, decodeErr
	}
	if !decoded.UsageValid {
		return decoded, errors.New("assistant provider usage was missing")
	}
	return decoded, nil
}

type assistantPlanMessageStream struct {
	raw     strings.Builder
	emitted string
	onDelta func(string) error
}

func (s *assistantPlanMessageStream) Append(delta string) error {
	if s == nil || delta == "" {
		return nil
	}
	s.raw.WriteString(delta)
	message, ready := assistantPartialFirstMessage([]byte(s.raw.String()))
	if !ready {
		return nil
	}
	if len([]byte(message)) > assistantMaxReplyBytes {
		return errors.New("assistant reply exceeded the size limit")
	}
	if !strings.HasPrefix(message, s.emitted) {
		return errors.New("assistant reply stream was inconsistent")
	}
	if len(message) == len(s.emitted) {
		return nil
	}
	next := message[len(s.emitted):]
	s.emitted = message
	if s.onDelta != nil {
		return s.onDelta(next)
	}
	return nil
}

func assistantPartialFirstMessage(input []byte) (string, bool) {
	position := skipAssistantJSONWhitespace(input, 0)
	if position >= len(input) || input[position] != '{' {
		return "", false
	}
	position = skipAssistantJSONWhitespace(input, position+1)
	key, next, complete, valid := readAssistantJSONPartialString(input, position)
	if !valid || !complete || key != "intent" {
		return "", false
	}
	position = skipAssistantJSONWhitespace(input, next)
	if position >= len(input) || input[position] != ':' {
		return "", false
	}
	position = skipAssistantJSONWhitespace(input, position+1)
	intent, next, complete, valid := readAssistantJSONPartialString(input, position)
	if !valid || !complete {
		return "", false
	}
	if intent != assistantIntentNone && intent != assistantIntentClarify {
		return "", true
	}
	position = skipAssistantJSONWhitespace(input, next)
	if position >= len(input) || input[position] != ',' {
		return "", false
	}
	position = skipAssistantJSONWhitespace(input, position+1)
	key, next, complete, valid = readAssistantJSONPartialString(input, position)
	if !valid || !complete || key != "message" {
		return "", false
	}
	position = skipAssistantJSONWhitespace(input, next)
	if position >= len(input) || input[position] != ':' {
		return "", false
	}
	position = skipAssistantJSONWhitespace(input, position+1)
	message, _, _, valid := readAssistantJSONPartialString(input, position)
	if !valid {
		return "", false
	}
	return message, true
}

func readAssistantJSONPartialString(input []byte, position int) (string, int, bool, bool) {
	if position >= len(input) || input[position] != '"' {
		return "", position, false, false
	}
	safeEnd := position + 1
	cursor := safeEnd
	for cursor < len(input) {
		current := input[cursor]
		switch {
		case current == '"':
			raw := input[position : cursor+1]
			var decoded string
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return "", cursor + 1, true, false
			}
			return decoded, cursor + 1, true, true
		case current == '\\':
			if cursor+1 >= len(input) {
				cursor = safeEnd
				goto partial
			}
			switch input[cursor+1] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				cursor += 2
			case 'u':
				if cursor+6 > len(input) {
					cursor = safeEnd
					goto partial
				}
				codeUnit, ok := assistantJSONCodeUnit(input[cursor+2 : cursor+6])
				if !ok {
					return "", cursor, false, false
				}
				if codeUnit >= 0xD800 && codeUnit <= 0xDBFF {
					if cursor+12 > len(input) {
						cursor = safeEnd
						goto partial
					}
					if input[cursor+6] == '\\' && input[cursor+7] == 'u' {
						low, ok := assistantJSONCodeUnit(input[cursor+8 : cursor+12])
						if !ok {
							return "", cursor, false, false
						}
						if low >= 0xDC00 && low <= 0xDFFF {
							cursor += 12
							safeEnd = cursor
							continue
						}
					}
				}
				cursor += 6
			default:
				return "", cursor, false, false
			}
			safeEnd = cursor
		case current < 0x20:
			return "", cursor, false, false
		case current < utf8.RuneSelf:
			cursor++
			safeEnd = cursor
		default:
			runeValue, size := utf8.DecodeRune(input[cursor:])
			if runeValue == utf8.RuneError && size == 1 {
				if !utf8.FullRune(input[cursor:]) {
					cursor = safeEnd
					goto partial
				}
				return "", cursor, false, false
			}
			cursor += size
			safeEnd = cursor
		}
	}
partial:
	raw := make([]byte, 0, safeEnd-position+1)
	raw = append(raw, input[position:safeEnd]...)
	raw = append(raw, '"')
	var decoded string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", safeEnd, false, false
	}
	return decoded, safeEnd, false, true
}

func assistantJSONCodeUnit(value []byte) (uint16, bool) {
	if len(value) != 4 {
		return 0, false
	}
	var codeUnit uint16
	for _, digit := range value {
		codeUnit <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			codeUnit |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			codeUnit |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			codeUnit |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return codeUnit, true
}

func skipAssistantJSONWhitespace(input []byte, position int) int {
	for position < len(input) {
		switch input[position] {
		case ' ', '\t', '\n', '\r':
			position++
		default:
			return position
		}
	}
	return position
}

func validAssistantReply(message string, allowEmpty bool) bool {
	if !utf8.ValidString(message) || len([]byte(message)) > assistantMaxReplyBytes {
		return false
	}
	if !allowEmpty && strings.TrimSpace(message) == "" {
		return false
	}
	for _, character := range message {
		if character == 0 || (character < 0x20 && character != '\n' && character != '\r' && character != '\t') {
			return false
		}
	}
	return true
}
