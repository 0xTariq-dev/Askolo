package product

import (
	"encoding/base64"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

func TestAttachmentFromObjectRoundTrip(t *testing.T) {
	prefix := "attachments/notes/user_1/42/"
	name := "meeting notes.txt"
	key := prefix + "file-1--" +
		base64.RawURLEncoding.EncodeToString([]byte("application/octet-stream")) + "--" +
		base64.RawURLEncoding.EncodeToString([]byte(name))
	modified := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	got, ok := attachmentFromObject(key, 512, modified, prefix)
	if !ok {
		t.Fatal("expected attachment object to parse")
	}
	if got.ID != "file-1" || got.FileName != name || got.SizeBytes != 512 || !got.CreatedAt.Equal(modified) {
		t.Fatalf("unexpected attachment: %#v", got)
	}
	if _, ok := attachmentFromObject(key, 512, modified, "attachments/notes/other/42/"); ok {
		t.Fatal("object outside the authorized prefix was accepted")
	}
}

func TestSafeAttachmentNameRejectsPathTraversalAndControls(t *testing.T) {
	for _, input := range []string{"", ".", "..", "bad\nname.txt"} {
		if got := safeAttachmentName(input); got != "" {
			t.Errorf("safeAttachmentName(%q) = %q, want rejection", input, got)
		}
	}
	if got := safeAttachmentName("../secret.txt"); got != "secret.txt" {
		t.Errorf("safeAttachmentName strips path components, got %q", got)
	}
	if got := safeAttachmentName("report.pdf"); got != "report.pdf" {
		t.Fatalf("safeAttachmentName returned %q", got)
	}
}

func TestFileRoutesRegisterWithoutConflictingPatterns(t *testing.T) {
	handler := NewHandler(config.Config{}, nil, nil, "")
	_ = handler.Routes()
}
