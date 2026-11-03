package r2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

func TestLiveJurisdictionBucketRoundTrip(t *testing.T) {
	if os.Getenv("ASKOLO_R2_LIVE_TESTS") != "yes" {
		t.Skip("set ASKOLO_R2_LIVE_TESTS=yes to exercise real R2 buckets")
	}

	accountID := os.Getenv("ASKOLO_R2_ACCOUNT_ID")
	accessKeyID := os.Getenv("ASKOLO_R2_REGIONAL_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("ASKOLO_R2_REGIONAL_SECRET_ACCESS_KEY")
	if accountID == "" || accessKeyID == "" || secretAccessKey == "" {
		t.Fatal("live R2 smoke test requires configured account and regional credentials")
	}

	for _, test := range []struct {
		jurisdiction string
		bucket       string
	}{
		{jurisdiction: "us", bucket: os.Getenv("ASKOLO_R2_US_BUCKET")},
		{jurisdiction: "eu", bucket: os.Getenv("ASKOLO_R2_EU_BUCKET")},
	} {
		t.Run(test.jurisdiction, func(t *testing.T) {
			client, err := New(config.R2Config{
				AccountID:       accountID,
				Bucket:          test.bucket,
				AccessKeyID:     accessKeyID,
				SecretAccessKey: secretAccessKey,
				Jurisdiction:    test.jurisdiction,
			})
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			prefix := fmt.Sprintf("agent-smoke-tests/%d/%s/", time.Now().UnixNano(), test.jurisdiction)
			key := prefix + "file.txt"
			content := []byte("Askolo regional R2 smoke test: " + test.jurisdiction)
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cleanupCancel()
				if err := client.Delete(cleanupCtx, key); err != nil {
					t.Errorf("temporary object cleanup failed: %v", err)
				}
			})
			if err := client.PutObject(ctx, key, "text/plain", content); err != nil {
				t.Fatalf("temporary object upload failed: %v", err)
			}

			objects, err := client.ListObjects(ctx, prefix, 10)
			if err != nil {
				t.Fatalf("temporary object listing failed: %v", err)
			}
			found := false
			for _, object := range objects {
				if object.Key == key {
					found = true
					break
				}
			}
			if !found {
				t.Fatal("uploaded object was not returned by the bucket list")
			}

			request, err := client.PresignGet(ctx, key, time.Minute)
			if err != nil {
				t.Fatalf("temporary object download could not be signed: %v", err)
			}
			httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := (&http.Client{Timeout: 15 * time.Second}).Do(httpRequest)
			if err != nil {
				t.Fatalf("temporary object download failed: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("temporary object download status = %d, want %d", response.StatusCode, http.StatusOK)
			}
			got, err := io.ReadAll(io.LimitReader(response.Body, int64(len(content))+1))
			if err != nil {
				t.Fatalf("temporary object response could not be read: %v", err)
			}
			if string(got) != string(content) {
				t.Fatal("temporary object content did not round-trip")
			}
		})
	}
}

func TestPresignPutUsesPrivateR2EndpointAndRequiresContentType(t *testing.T) {
	client, err := New(config.R2Config{
		AccountID:       "11111111111111111111111111111111",
		Bucket:          "askolo-test-bucket",
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
	if parsed.Path != "/askolo-test-bucket/spaces/space-123/files/file-456" {
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
		Bucket:          "askolo-test-bucket",
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
		Bucket:          "askolo-test-bucket",
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
