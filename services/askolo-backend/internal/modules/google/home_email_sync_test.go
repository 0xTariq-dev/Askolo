package google

import (
	"io"
	"net/http"
	"strings"
	"testing"
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

func homeSyncResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
