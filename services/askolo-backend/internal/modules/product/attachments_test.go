package product

import (
	"context"
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

func TestAttachmentFromObjectParsesDelimiterInsideIdentifier(t *testing.T) {
	prefix := "attachments/notes/user_1/42/"
	encodedType := base64.RawURLEncoding.EncodeToString([]byte("application/octet-stream"))
	encodedName := base64.RawURLEncoding.EncodeToString([]byte("report.txt"))
	key := prefix + "file--segment--" + encodedType + "--" + encodedName

	got, ok := attachmentFromObject(key, 12, time.Time{}, prefix)
	if !ok {
		t.Fatal("expected attachment with a delimiter-containing identifier to parse")
	}
	if got.ID != "file--segment" || got.FileName != "report.txt" {
		t.Fatalf("parsed attachment = %#v", got)
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

func TestStorageForUserUsesResolvedJurisdictionAndDoesNotCrossFallback(t *testing.T) {
	us := newAttachmentIntegrationStorage()
	eu := newAttachmentIntegrationStorage()
	handler := NewHandler(config.Config{}, nil, nil, "")
	handler.fileStorageByJurisdiction = map[string]privateFileStorage{
		"us": us,
		"eu": eu,
	}
	handler.SetStorageJurisdictionResolver(func(_ context.Context, userID string) (string, error) {
		if userID == "eu-user" {
			return "eu", nil
		}
		return "us", nil
	})

	target, err := handler.storageForUser(context.Background(), "eu-user")
	if err != nil {
		t.Fatal(err)
	}
	if target.jurisdiction != "eu" || target.storage != eu {
		t.Fatalf("EU user was routed to %#v, want EU storage", target)
	}

	handler.fileStorageByJurisdiction = map[string]privateFileStorage{"us": us}
	if _, err := handler.storageForUser(context.Background(), "eu-user"); err == nil {
		t.Fatal("missing EU storage silently fell back to US")
	}
}

func TestProfileImageRegionMigratesLegacyMarkersToUS(t *testing.T) {
	us := newAttachmentIntegrationStorage()
	eu := newAttachmentIntegrationStorage()
	handler := NewHandler(config.Config{}, nil, nil, "")
	handler.fileStorageByJurisdiction = map[string]privateFileStorage{"us": us, "eu": eu}

	oldURLTarget, err := handler.profileImageStorageTarget("/api/user/profile-image?version=1")
	if err != nil {
		t.Fatal(err)
	}
	if oldURLTarget.jurisdiction != "us" || oldURLTarget.storage != us {
		t.Fatalf("old profile image URL routed to %#v, want US storage", oldURLTarget)
	}
	legacyMarkedTarget, err := handler.profileImageStorageTarget("/api/user/profile-image?version=1&region=legacy")
	if err != nil {
		t.Fatal(err)
	}
	if legacyMarkedTarget.jurisdiction != "us" || legacyMarkedTarget.storage != us {
		t.Fatalf("legacy-marked profile image URL routed to %#v, want US storage", legacyMarkedTarget)
	}
	euTarget, err := handler.profileImageStorageTarget("/api/user/profile-image?version=2&region=eu")
	if err != nil {
		t.Fatal(err)
	}
	if euTarget.jurisdiction != "eu" || euTarget.storage != eu {
		t.Fatalf("EU profile image URL routed to %#v, want EU storage", euTarget)
	}
	if _, err := handler.profileImageStorageTarget("/api/user/profile-image?region=weur"); err == nil {
		t.Fatal("a location hint was accepted as a jurisdiction")
	}
}

func TestListFileObjectsReadsAllConfiguredJurisdictions(t *testing.T) {
	us := newAttachmentIntegrationStorage()
	eu := newAttachmentIntegrationStorage()
	us.putTestObject("attachments/tasks/space/task/us-file")
	eu.putTestObject("attachments/tasks/space/task/eu-file")

	handler := NewHandler(config.Config{}, nil, nil, "")
	handler.fileStorageByJurisdiction = map[string]privateFileStorage{"us": us, "eu": eu}
	objects, err := handler.listFileObjects(
		context.Background(),
		"attachments/tasks/space/task/",
		maxAttachments+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 {
		t.Fatalf("found %d stored objects, want one from each configured jurisdiction", len(objects))
	}
}

func TestFileRoutesRegisterWithoutConflictingPatterns(t *testing.T) {
	handler := NewHandler(config.Config{}, nil, nil, "")
	_ = handler.Routes()
}
