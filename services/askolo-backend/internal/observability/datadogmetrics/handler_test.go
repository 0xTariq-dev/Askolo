package datadogmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

type recordingSender struct {
	payloads chan metricPayload
}

func (s *recordingSender) submit(_ context.Context, payload metricPayload) (int, error) {
	s.payloads <- payload
	return http.StatusAccepted, nil
}

func TestRouteLabelsAreBoundedAndDoNotIncludePathValues(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		method string
		status int
		want   groupKey
	}{
		{
			name:   "health",
			path:   "/readyz",
			method: http.MethodGet,
			status: http.StatusOK,
			want:   groupKey{routeGroup: "health", method: "GET", statusClass: "2xx"},
		},
		{
			name:   "API path with an identifier",
			path:   "/api/v2/spaces/private-space-id/tasks",
			method: http.MethodPost,
			status: http.StatusInternalServerError,
			want:   groupKey{routeGroup: "api_v2", method: "POST", statusClass: "5xx"},
		},
		{
			name:   "unknown method",
			path:   "/anything",
			method: "CUSTOM",
			status: 0,
			want:   groupKey{routeGroup: "web", method: "OTHER", statusClass: "unknown"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := groupKey{
				routeGroup:  routeGroup(test.path),
				method:      methodLabel(test.method),
				statusClass: statusClass(test.status),
			}
			if got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestObserveAndFlushSendsOnlyAggregatedSafeDimensions(t *testing.T) {
	sender := &recordingSender{payloads: make(chan metricPayload, 1)}
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	handler := newHandler(Config{
		Environment: "production",
		Service:     "askolo-backend",
		ReadDatabasePoolStats: func() DatabasePoolSnapshot {
			return DatabasePoolSnapshot{
				TotalConnections:    4,
				AcquiredConnections: 2,
				IdleConnections:     2,
				MaxConnections:      12,
				AcquireCount:        30,
				AcquireDurationMS:   11,
			}
		},
	}, logger, sender)

	handler.ObserveHTTPRequest(http.MethodGet, "/api/v2/spaces/private-space-id?token=not-forwarded", http.StatusOK, 18*time.Millisecond)
	handler.ObserveHTTPRequest(http.MethodGet, "/api/v2/spaces/another-id?email=not-forwarded", http.StatusServiceUnavailable, 42*time.Millisecond)

	handler.flush()
	payload := <-sender.payloads
	requestCountSeries := 0
	for _, series := range payload.counts.Series {
		if series.Metric == "askolo.http.server.request_count_30s" {
			requestCountSeries++
		}
	}
	if len(payload.distributions.Series) != 2 || requestCountSeries != 2 {
		t.Fatalf("expected two status-grouped request series, got %d distributions and %d request counts",
			len(payload.distributions.Series), requestCountSeries)
	}
	distributions, err := json.Marshal(payload.distributions)
	if err != nil {
		t.Fatalf("marshal distribution payload: %v", err)
	}
	counts, err := json.Marshal(payload.counts)
	if err != nil {
		t.Fatalf("marshal count payload: %v", err)
	}
	encoded := string(distributions) + string(counts)
	for _, forbidden := range []string{"private-space-id", "another-id", "not-forwarded", "token=", "email="} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("payload contains private request data %q", forbidden)
		}
	}
	if !strings.Contains(encoded, "route_group:api_v2") ||
		!strings.Contains(encoded, "status_class:2xx") ||
		!strings.Contains(encoded, "status_class:5xx") {
		t.Fatalf("payload is missing expected bounded tags: %s", encoded)
	}
	if !strings.Contains(encoded, requestDurationMetric) ||
		!strings.Contains(encoded, "askolo.http.server.request_count_30s") ||
		!strings.Contains(encoded, "askolo.go.goroutines") ||
		!strings.Contains(encoded, "askolo.db.pool.total_connections") {
		t.Fatalf("payload is missing expected metrics: %s", encoded)
	}
}

func TestShutdownFlushesPendingSamples(t *testing.T) {
	sender := &recordingSender{payloads: make(chan metricPayload, 1)}
	handler := newHandler(Config{
		Environment: "production",
		Service:     "askolo-backend",
	}, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), sender)
	go handler.run()
	handler.ObserveHTTPRequest(http.MethodGet, "/healthz", http.StatusOK, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := handler.shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if got := len((<-sender.payloads).distributions.Series); got != 1 {
		t.Fatalf("expected one flushed distribution series, got %d", got)
	}
}

func TestObserveKeepsAReservoirForHighVolumeGroups(t *testing.T) {
	handler := newHandler(Config{
		Environment: "production",
		Service:     "askolo-backend",
	}, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), &recordingSender{payloads: make(chan metricPayload, 1)})
	handler.random = rand.New(rand.NewSource(1))
	for i := 0; i < maxSamplesPerGroup+100; i++ {
		handler.ObserveHTTPRequest(http.MethodGet, "/api/v2/tasks", http.StatusOK, time.Duration(i)*time.Microsecond)
	}
	bucket := handler.groups[groupKey{routeGroup: "api_v2", method: "GET", statusClass: "2xx"}]
	if bucket == nil || bucket.count != maxSamplesPerGroup+100 {
		t.Fatalf("expected all requests counted, got %+v", bucket)
	}
	if len(bucket.durations) != maxSamplesPerGroup {
		t.Fatalf("expected bounded sample of %d, got %d", maxSamplesPerGroup, len(bucket.durations))
	}
}

func TestDatadogLiveRequestMetricSmoke(t *testing.T) {
	if os.Getenv("ASKOLO_DATADOG_INTAKE_SMOKE") != "1" {
		t.Skip("set ASKOLO_DATADOG_INTAKE_SMOKE=1 to send a synthetic metric")
	}
	cfg := Config{
		APIKey:      os.Getenv("DD_API_KEY"),
		Site:        os.Getenv("DD_SITE"),
		Environment: os.Getenv("ASKOLO_ENVIRONMENT"),
		Service:     "askolo-backend",
	}
	if cfg.APIKey == "" || cfg.Site == "" || !supportedEnvironment(cfg.Environment) {
		t.Skip("Datadog API credentials and a supported app environment are required")
	}
	client := datadog.NewAPIClient(datadog.NewConfiguration())
	sender := &apiSender{
		site:      cfg.Site,
		apiKey:    cfg.APIKey,
		metricsV1: datadogV1.NewMetricsApi(client),
		metricsV2: datadogV2.NewMetricsApi(client),
	}
	payload := buildPayload(cfg, map[groupKey]aggregate{
		{routeGroup: "api", method: "GET", statusClass: "2xx"}: {
			count:     1,
			durations: []float64{1},
		},
	}, time.Now().UTC())
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	status, err := sender.submit(ctx, payload)
	if err != nil || status < 200 || status >= 300 {
		t.Fatalf("Datadog metric intake returned status %d", status)
	}
}

func TestDatadogLiveMFAProductionAlertRecoverySmoke(t *testing.T) {
	if os.Getenv("ASKOLO_DATADOG_PRODUCTION_ALERT_SMOKE") != "1" {
		t.Skip("set ASKOLO_DATADOG_PRODUCTION_ALERT_SMOKE=1 to create and recover one Production test page")
	}
	apiKey := os.Getenv("DD_API_KEY")
	site := os.Getenv("DD_SITE")
	if apiKey == "" || !supportedSite(site) {
		t.Skip("Datadog API credentials and site are required")
	}

	client := datadog.NewAPIClient(datadog.NewConfiguration())
	sender := &apiSender{
		site:      site,
		apiKey:    apiKey,
		metricsV1: datadogV1.NewMetricsApi(client),
		metricsV2: datadogV2.NewMetricsApi(client),
	}
	sendGauge := func(value float64) error {
		metricType := datadogV2.METRICINTAKETYPE_GAUGE
		payload := metricPayload{
			counts: datadogV2.MetricPayload{
				Series: []datadogV2.MetricSeries{{
					Metric: "askolo.mfa.security.alert_active",
					Type:   &metricType,
					Points: []datadogV2.MetricPoint{{
						Timestamp: datadog.PtrInt64(time.Now().UTC().Unix()),
						Value:     datadog.PtrFloat64(value),
					}},
					Tags: []string{
						"environment:production",
						"operation:mfa_security_spike",
						"alert_reasons:failure_events",
					},
				}},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		status, err := sender.submit(ctx, payload)
		if err != nil {
			return err
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("Datadog metric intake returned status %d", status)
		}
		return nil
	}

	active := false
	defer func() {
		if active {
			_ = sendGauge(0)
		}
	}()
	for i := 0; i < 4; i++ {
		if err := sendGauge(1); err != nil {
			t.Fatalf("submit synthetic active MFA signal: %v", err)
		}
		active = true
		if i < 3 {
			time.Sleep(20 * time.Second)
		}
	}
	time.Sleep(2 * time.Second)
	if err := sendGauge(0); err != nil {
		t.Fatalf("submit synthetic MFA recovery signal: %v", err)
	}
	active = false
	time.Sleep(75 * time.Second)
}

func TestDatadogLiveMFAProductionRecoverySignal(t *testing.T) {
	if os.Getenv("ASKOLO_DATADOG_PRODUCTION_RECOVERY_SMOKE") != "1" {
		t.Skip("set ASKOLO_DATADOG_PRODUCTION_RECOVERY_SMOKE=1 to submit a Production recovery signal")
	}
	apiKey := os.Getenv("DD_API_KEY")
	site := os.Getenv("DD_SITE")
	if apiKey == "" || !supportedSite(site) {
		t.Skip("Datadog API credentials and site are required")
	}

	client := datadog.NewAPIClient(datadog.NewConfiguration())
	sender := &apiSender{
		site:      site,
		apiKey:    apiKey,
		metricsV1: datadogV1.NewMetricsApi(client),
		metricsV2: datadogV2.NewMetricsApi(client),
	}
	metricType := datadogV2.METRICINTAKETYPE_GAUGE
	payload := metricPayload{
		counts: datadogV2.MetricPayload{
			Series: []datadogV2.MetricSeries{{
				Metric: "askolo.mfa.security.alert_active",
				Type:   &metricType,
				Points: []datadogV2.MetricPoint{{
					Timestamp: datadog.PtrInt64(time.Now().UTC().Unix()),
					Value:     datadog.PtrFloat64(0),
				}},
				Tags: []string{
					"environment:production",
					"operation:mfa_security_spike",
					"alert_reasons:failure_events",
				},
			}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	status, err := sender.submit(ctx, payload)
	if err != nil || status < 200 || status >= 300 {
		t.Fatalf("Datadog recovery metric intake returned status %d", status)
	}
}
