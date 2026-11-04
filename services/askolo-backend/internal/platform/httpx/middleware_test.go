package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type observedRequest struct {
	method   string
	path     string
	status   int
	duration time.Duration
}

type requestObserverFunc func(method, path string, status int, duration time.Duration)

func (f requestObserverFunc) ObserveHTTPRequest(method, path string, status int, duration time.Duration) {
	f(method, path, status, duration)
}

func TestObserveRequestsRecordsCompletionWithoutQueryString(t *testing.T) {
	var got observedRequest
	observer := requestObserverFunc(func(method, path string, status int, duration time.Duration) {
		got = observedRequest{method: method, path: path, status: status, duration: duration}
	})
	handler := ObserveRequests(observer, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/spaces/private-id?token=not-forwarded", nil)
	handler.ServeHTTP(recorder, request)

	if got.method != http.MethodPost || got.path != "/api/v2/spaces/private-id" ||
		got.status != http.StatusServiceUnavailable || got.duration <= 0 {
		t.Fatalf("unexpected request observation: %+v", got)
	}
}
