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
