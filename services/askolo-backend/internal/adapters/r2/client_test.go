package r2

import (
	"context"
	"net/url"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

func TestPresignPutUsesPrivateR2EndpointAndRequiresContentType(t *testing.T) {
	client, err := New(config.R2Config{
		AccountID:       "11111111111111111111111111111111",
		Bucket:          "askolo-development-files",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := client.PresignPut(context.Background(), "spaces/space-123/files/file-456", "application/pdf", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(request.URL)
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != "PUT" || parsed.Host != "11111111111111111111111111111111.r2.cloudflarestorage.com" {
		t.Fatalf("request method or host is invalid")
	}
	if parsed.Path != "/askolo-development-files/spaces/space-123/files/file-456" {
		t.Fatalf("signed path = %q", parsed.Path)
	}
	if request.Headers.Get("Content-Type") != "application/pdf" {
		t.Fatal("content type was not included in signed headers")
	}
	if parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatal("presigned request has no signature")
	}
}

func TestJurisdictionUsesMatchingR2Endpoint(t *testing.T) {
	for _, jurisdiction := range []string{"us", "eu"} {
		t.Run(jurisdiction, func(t *testing.T) {
			client, err := New(config.R2Config{
				AccountID:       "11111111111111111111111111111111",
				Bucket:          "askolo-production-" + jurisdiction + "-files",
				AccessKeyID:     "test-access-key",
				SecretAccessKey: "test-secret-key",
				Jurisdiction:    jurisdiction,
			})
			if err != nil {
				t.Fatal(err)
			}
			request, err := client.PresignGet(
				context.Background(),
				"spaces/space-123/files/file-456",
				time.Minute,
			)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(request.URL)
			if err != nil {
				t.Fatal(err)
			}
			wantHost := "11111111111111111111111111111111." + jurisdiction + ".r2.cloudflarestorage.com"
			if parsed.Host != wantHost {
				t.Fatalf("host = %q, want jurisdiction endpoint %q", parsed.Host, wantHost)
			}
		})
	}
}

func TestRejectsUnknownR2Jurisdiction(t *testing.T) {
	if _, err := New(config.R2Config{
		AccountID:       "11111111111111111111111111111111",
		Bucket:          "askolo-production-us-files",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Jurisdiction:    "weur",
	}); err == nil {
		t.Fatal("location hint was accepted as a jurisdiction")
	}
}

func TestPresignRejectsUnsafeKeysAndLongExpiry(t *testing.T) {
	client, err := New(config.R2Config{
		AccountID:       "11111111111111111111111111111111",
		Bucket:          "askolo-development-files",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../secret", "/absolute", "a//b", "a\\b", "bad key"} {
		if _, err := client.PresignGet(context.Background(), key, time.Minute); err == nil {
			t.Errorf("unsafe key %q was accepted", key)
		}
	}
	if _, err := client.PresignGet(context.Background(), "spaces/space-123/files/file-456", maxPresignTTL+time.Second); err == nil {
		t.Fatal("presign TTL above the maximum was accepted")
	}
}

func TestPresignGetAttachmentSetsSafeDownloadDisposition(t *testing.T) {
	client, err := New(config.R2Config{
		AccountID:       "11111111111111111111111111111111",
		Bucket:          "askolo-development-files",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := client.PresignGetAttachment(
		context.Background(),
		"attachments/notes/user_1/42/file-1",
		time.Minute,
		"meeting notes.txt",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(request.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("response-content-disposition"); got != `attachment; filename="meeting notes.txt"` {
		t.Fatalf("content disposition = %q", got)
	}
	for _, name := range []string{"../secret.txt", "bad\\name.txt", "bad\nname.txt"} {
		if _, err := client.PresignGetAttachment(
			context.Background(),
			"attachments/notes/user_1/42/file-1",
			time.Minute,
			name,
		); err == nil {
			t.Errorf("unsafe download filename %q was accepted", name)
		}
	}
}
