package google

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type homeSyncRoundTripper func(*http.Request) (*http.Response, error)

func (f homeSyncRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestListHomeInboxThreadsUsesBoundedInboxRequest(t *testing.T) {
	handler := &Handler{client: &http.Client{Transport: homeSyncRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("labelIds") != "INBOX" || request.URL.Query().Get("maxResults") != "500" {
			t.Fatalf("unexpected Gmail thread query: %s", request.URL.RawQuery)
		}
		return homeSyncResponse(`{"threads":[{"id":"thread-a"}]}`), nil
	})}}

	threads, err := handler.listHomeInboxThreads(t.Context(), "access-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0] != "thread-a" {
		t.Fatalf("thread IDs = %v, want [thread-a]", threads)
	}
}

func TestListHomeInboxThreadsRejectsPartialResults(t *testing.T) {
	handler := &Handler{client: &http.Client{Transport: homeSyncRoundTripper(func(*http.Request) (*http.Response, error) {
		return homeSyncResponse(`{"threads":[{"id":"thread-a"}],"nextPageToken":"next"}`), nil
	})}}

	if _, err := handler.listHomeInboxThreads(t.Context(), "access-token"); err == nil {
		t.Fatal("expected paginated Gmail result to be rejected rather than saved as a partial summary")
	}
}

func TestListHomeSummaryThreadPageUsesWindowAndResumes(t *testing.T) {
	cutoff := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	wantQuery := fmt.Sprintf("after:%d {in:inbox is:starred is:important}", cutoff.Unix())
	handler := &Handler{client: &http.Client{Transport: homeSyncRoundTripper(func(request *http.Request) (*http.Response, error) {
		query := request.URL.Query()
		if query.Get("maxResults") != "500" ||
			query.Get("pageToken") != "resume-here" ||
			query.Get("q") != wantQuery {
			t.Fatalf("unexpected summary page query: %s", request.URL.RawQuery)
		}
		return homeSyncResponse(`{"threads":[{"id":"thread-a"}]}`), nil
	})}}
	page, err := handler.listHomeSummaryThreadPage(t.Context(), "access-token", cutoff, "resume-here")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Threads) != 1 || page.Threads[0].ID != "thread-a" {
		t.Fatalf("threads = %+v", page.Threads)
	}
}

func TestGmailHistoryPageBuildsDeduplicatedThreadSet(t *testing.T) {
	page := gmailHistoryPage{History: []gmailHistoryRecord{
		{
			Messages: []gmailHistoryMessageRef{{ThreadID: "thread-a"}},
			MessagesAdded: []struct {
				Message gmailHistoryMessageRef `json:"message"`
			}{{Message: gmailHistoryMessageRef{ThreadID: "thread-b"}}},
			LabelsRemoved: []struct {
				Message gmailHistoryMessageRef `json:"message"`
			}{{Message: gmailHistoryMessageRef{ThreadID: "thread-a"}}},
		},
	}}
	got := page.threadIDs()
	if len(got) != 2 {
		t.Fatalf("thread IDs = %v, want two unique threads", got)
	}
	seen := map[string]bool{}
	for _, id := range got {
		seen[id] = true
	}
	if !seen["thread-a"] || !seen["thread-b"] {
		t.Fatalf("thread IDs = %v", got)
	}
}

func TestGmailPermissionFailuresAreDistinctFromQuotaFailures(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		body           string
		actionRequired bool
	}{
		{
			name: "missing Gmail scope", status: http.StatusForbidden,
			body:           `{"error":{"errors":[{"reason":"insufficientPermissions"}]}}`,
			actionRequired: true,
		},
		{
			name: "Gmail quota", status: http.StatusForbidden,
			body: `{"error":{"errors":[{"reason":"userRateLimitExceeded"}]}}`,
		},
		{
			name: "provider throttling", status: http.StatusTooManyRequests,
			body: `{"error":{"errors":[{"reason":"rateLimitExceeded"}]}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &Handler{client: &http.Client{Transport: homeSyncRoundTripper(
				func(*http.Request) (*http.Response, error) {
					response := homeSyncResponse(test.body)
					response.StatusCode = test.status
					return response, nil
				},
			)}}
			_, err := handler.googleRequest(t.Context(), http.MethodGet,
				"https://gmail.googleapis.com/gmail/v1/users/me/profile", "access-token", nil)
			if err == nil {
				t.Fatal("expected provider error")
			}
			if got := isHomeEmailActionRequired(err); got != test.actionRequired {
				t.Fatalf("action required = %v, want %v", got, test.actionRequired)
			}
		})
	}
}

func homeSyncResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
