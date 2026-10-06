package product

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	errAssistantProcessingConsentRequired = errors.New("assistant processing consent is required")
	errAssistantRedactedTranscriptInvalid = errors.New("assistant transcript is invalid after redaction")

	assistantEmailPattern      = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	assistantIBANPattern       = regexp.MustCompile(`(?i)\b[A-Z]{2}[0-9]{2}(?:[ ]?[A-Z0-9]){11,30}\b`)
	assistantCardPattern       = regexp.MustCompile(`\b(?:[0-9][ -]?){13,19}\b`)
	assistantIDPattern         = regexp.MustCompile(`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`)
	assistantPhonePattern      = regexp.MustCompile(`(?:\+|00)?[0-9][0-9(). -]{7,}[0-9]`)
	assistantCredentialPattern = regexp.MustCompile(
		`(?i)\b(password|passcode|pin|verification code|access token|api key|secret)\b\s*(?:is|:|=)?\s*["']?[^\s,;]+`,
	)
	assistantBirthDatePattern = regexp.MustCompile(
		`(?i)\b(date of birth|birth date|birthday|dob)\b\s*(?:is|:|=)?\s*(?:[0-9]{4}[-/][0-9]{1,2}[-/][0-9]{1,2}|[0-9]{1,2}[-/][0-9]{1,2}[-/][0-9]{2,4}|[A-Z]{3,9}\s+[0-9]{1,2},?\s+[0-9]{4}|[0-9]{1,2}\s+[A-Z]{3,9}\s+[0-9]{4})`,
	)
	assistantGovernmentIDPattern = regexp.MustCompile(
		`(?i)\b(passport|national id|government id|social security)\b\s*(?:number|no\.?)?\s*(?:is|:|=)?\s*[A-Z0-9 -]{5,24}`,
	)
	assistantAddressPattern = regexp.MustCompile(
		`(?i)\b(?:home address|street address|address|عنوان المنزل|العنوان)\b\s*(?:is|:|=)?\s*[^.!?\n]{1,100}`,
	)
	assistantOperationalDateTimePattern = regexp.MustCompile(
		`\b(?:[0-9]{4}[-/][0-9]{1,2}[-/][0-9]{1,2}|[0-9]{1,2}[-/][0-9]{1,2}[-/][0-9]{2,4})(?:[ T]+[0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?)?\b|\b[0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?\b|\b[0-9]+\s*(?:seconds?|minutes?|hours?|days?|weeks?|months?|years?)\b`,
	)
)

func (h *Handler) assistantFacingTranscript(ctx context.Context, userID, transcript string) (string, error) {
	if h == nil || h.store == nil {
		return "", errors.New("assistant privacy preferences are unavailable")
	}
	consent, _, err := h.store.AssistantProcessingConsent(ctx, userID)
	if err != nil {
		return "", err
	}
	if !consent {
		return "", errAssistantProcessingConsentRequired
	}

	filtered := redactAssistantSensitiveValues(transcript)
	cleaned, ok := preflightAssistantTranscript(filtered)
	if !ok || !utf8.ValidString(cleaned) {
		return "", errAssistantRedactedTranscriptInvalid
	}
	return cleaned, nil
}

func redactAssistantSensitiveValues(input string) string {
	input = assistantCredentialPattern.ReplaceAllString(input, "$1 [REDACTED]")
	input = assistantBirthDatePattern.ReplaceAllString(input, "$1 [REDACTED]")
	input = assistantAddressPattern.ReplaceAllString(input, "[REDACTED_ADDRESS]")
	input = assistantGovernmentIDPattern.ReplaceAllString(input, "$1 [REDACTED_GOVERNMENT_ID]")
	input = assistantEmailPattern.ReplaceAllString(input, "[REDACTED_EMAIL]")
	input = assistantIBANPattern.ReplaceAllString(input, "[REDACTED_FINANCIAL_ID]")
	input = assistantCardPattern.ReplaceAllStringFunc(input, func(value string) string {
		digits := digitsIn(value)
		if digits >= 13 && digits <= 19 && passesLuhn(value) {
			return "[REDACTED_FINANCIAL_ID]"
		}
		return value
	})
	input = assistantIDPattern.ReplaceAllString(input, "[REDACTED_GOVERNMENT_ID]")

	// Keep scheduling information intact before applying the broad phone
	// matcher. Values are restored after contact and financial identifiers are
	// filtered, so dates of birth are already removed above.
	protected := make([]string, 0, 4)
	input = assistantOperationalDateTimePattern.ReplaceAllStringFunc(input, func(value string) string {
		protected = append(protected, value)
		return "\x00ASKOLO_OPERATIONAL_VALUE_" + strconv.Itoa(len(protected)-1) + "\x00"
	})
	input = assistantPhonePattern.ReplaceAllStringFunc(input, func(value string) string {
		digits := digitsIn(value)
		if digits >= 9 && digits <= 15 {
			return "[REDACTED_PHONE]"
		}
		return value
	})

	for index, value := range protected {
		marker := "\x00ASKOLO_OPERATIONAL_VALUE_" + strconv.Itoa(index) + "\x00"
		input = strings.ReplaceAll(input, marker, value)
	}
	return input
}

func digitsIn(value string) int {
	digits := 0
	for _, character := range value {
		if character >= '0' && character <= '9' {
			digits++
		}
	}
	return digits
}

func passesLuhn(value string) bool {
	sum := 0
	alternate := false
	for index := len(value) - 1; index >= 0; index-- {
		character := value[index]
		if character < '0' || character > '9' {
			continue
		}
		digit := int(character - '0')
		if alternate {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		alternate = !alternate
	}
	return sum%10 == 0
}
