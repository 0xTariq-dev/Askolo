package datadogmfa

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"askolo/backend/internal/modules/auth"
)

type captureSender struct {
	mu          sync.Mutex
	submissions []submission
}

func (s *captureSender) send(_ context.Context, item submission) delivery {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submissions = append(s.submissions, item)
	return delivery{}
}

func (s *captureSender) snapshot() []submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]submission(nil), s.submissions...)
}

func newTestHandler(sender telemetrySender, now func() time.Time) *Handler {
	next := slog.NewJSONHandler(io.Discard, nil)
	core := &handlerCore{
		config: Config{
			APIKey:      "test-api-key",
			Site:        "us5.datadoghq.com",
			Environment: "development",
		},
		sender:     sender,
		reporter:   next,
		queue:      make(chan submission, telemetryQueueSize),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		lastMetric: make(map[string]time.Time),
		now:        now,
	}
	go core.run()
	return &Handler{next: next, core: core}
}

func shutdownTestHandler(t *testing.T, handler *Handler) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := handler.core.shutdown(ctx); err != nil {
		t.Fatalf("shutdown Datadog test handler: %v", err)
	}
}

func TestLogForwardingOnlyIncludesAllowlistedMFAFields(t *testing.T) {
	sender := &captureSender{}
	now := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	handler := newTestHandler(sender, func() time.Time { return now })
	record := slog.NewRecord(now, slog.LevelWarn, alertMessage, 0)
	record.AddAttrs(
		slog.Bool("alert", true),
		slog.Bool("recovery", false),
		slog.String("operation", alertOperation),
		slog.String("environment", "development"),
		slog.Int("window_minutes", 15),
		slog.Int64("failure_events", 20),
		slog.Int64("replay_events", 0),
		slog.Int64("lockout_events", 0),
		slog.Int64("decryption_failure_events", 0),
		slog.Int64("recovery_support_requests", 0),
		slog.Int64("recovery_support_verification_failures", 0),
		slog.Int64("recovery_support_rate_limited", 0),
		slog.Int64("recovery_support_session_revocation_failures", 0),
		slog.Int64("affected_users", 4),
		slog.String("alert_reasons", "failure_events"),
		slog.String("user_id", "must-not-be-forwarded"),
		slog.String("request_id", "must-not-be-forwarded"),
		slog.String("email", "must-not-be-forwarded"),
	)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() = %v", err)
	}
	shutdownTestHandler(t, handler)

	items := sender.snapshot()
	if len(items) != 1 || items[0].logItem == nil {
		t.Fatalf("submissions = %+v, want one allowlisted log", items)
	}
	properties := items[0].logItem.AdditionalProperties
	for _, forbidden := range []string{"user_id", "request_id", "email"} {
		if _, exists := properties[forbidden]; exists {
			t.Fatalf("Datadog log includes forbidden field %q: %+v", forbidden, properties)
		}
	}
	allowed := map[string]struct{}{
		"environment":    {},
		"operation":      {},
		"alert_reasons":  {},
		"alert":          {},
		"recovery":       {},
		"window_minutes": {},
	}
	for _, field := range aggregateCountFields {
		allowed[field] = struct{}{}
	}
	for field := range properties {
		if _, exists := allowed[field]; !exists {
			t.Fatalf("Datadog log includes non-allowlisted field %q", field)
		}
	}
	for _, required := range []string{"environment", "operation", "alert_reasons", "failure_events", "affected_users"} {
		if _, exists := properties[required]; !exists {
			t.Fatalf("Datadog log is missing %q: %+v", required, properties)
		}
	}
	if properties["environment"] != "development" ||
		properties["operation"] != alertOperation ||
		properties["alert_reasons"] != "failure_events" ||
		properties["alert"] != true ||
		properties["recovery"] != false ||
		properties["window_minutes"] != int64(15) {
		t.Fatalf("Datadog labels = %+v, want fixed development MFA labels", properties)
	}
}

func TestLogForwardingIncludesExplicitRecoveryState(t *testing.T) {
	item := buildLogItem(alertEvent{
		message:     recoveryMessage,
		environment: "production",
		reasonSet:   "recovery_support_rate_limited",
		window:      15,
		recovery:    true,
	})
	properties := item.AdditionalProperties
	if properties["alert"] != false || properties["recovery"] != true ||
		properties["window_minutes"] != int64(15) {
		t.Fatalf("recovery log state = %+v, want alert=false recovery=true window_minutes=15", properties)
	}
	for _, forbidden := range []string{"user_id", "request_id", "email", "code", "secret"} {
		if _, exists := properties[forbidden]; exists {
			t.Fatalf("Datadog recovery log includes forbidden field %q", forbidden)
		}
	}
}

func TestMetricHeartbeatKeepsAlertActiveAndRecoveryUsesSameGroup(t *testing.T) {
	sender := &captureSender{}
	now := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	handler := newTestHandler(sender, func() time.Time { return now })
	signal := auth.MFASecurityReadiness{
		Status:        "available",
		Environment:   "development",
		WindowMinutes: 15,
		Alert:         true,
		AlertReasons:  []string{"failure_events"},
	}

	handler.ObserveMFASecurity(context.Background(), signal)
	now = now.Add(10 * time.Second)
	handler.ObserveMFASecurity(context.Background(), signal)
	now = now.Add(activeMetricHeartbeat)
	handler.ObserveMFASecurity(context.Background(), signal)

	recovered := signal
	recovered.Alert = false
	handler.ObserveMFASecurity(context.Background(), recovered)
	unavailable := recovered
	unavailable.Status = "unavailable"
	handler.ObserveMFASecurity(context.Background(), unavailable)
	shutdownTestHandler(t, handler)

	items := sender.snapshot()
	if len(items) != 3 {
		t.Fatalf("metric submissions = %d, want initial state, heartbeat, and recovery", len(items))
	}
	wantValues := []float64{1, 1, 0}
	for index, item := range items {
		if item.metricPayload == nil || len(item.metricPayload.Series) != 1 {
			t.Fatalf("submission %d has no single metric series: %+v", index, item)
		}
		series := item.metricPayload.Series[0]
		if got := series.Points[0].GetValue(); got != wantValues[index] {
			t.Fatalf("metric point %d = %v, want %v", index, got, wantValues[index])
		}
		if series.Metric != activeMetricName {
			t.Fatalf("metric name = %q, want %q", series.Metric, activeMetricName)
		}
		if len(series.Tags) != 3 ||
			series.Tags[0] != "environment:development" ||
			series.Tags[1] != "operation:mfa_security_spike" ||
			series.Tags[2] != "alert_reasons:failure_events" {
			t.Fatalf("metric tags = %v, want stable environment/operation/reason grouping", series.Tags)
		}
	}
}

func TestMetricGroupingIsStableAcrossBackendInstances(t *testing.T) {
	firstSender := &captureSender{}
	secondSender := &captureSender{}
	now := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	first := newTestHandler(firstSender, func() time.Time { return now })
	second := newTestHandler(secondSender, func() time.Time { return now })
	signal := auth.MFASecurityReadiness{
		Status:        "available",
		Environment:   "production",
		WindowMinutes: 15,
		Alert:         true,
		AlertReasons:  []string{"failure_events", "replay_events"},
	}
	first.core.config.Environment = "production"
	second.core.config.Environment = "production"
	signal.Environment = "production"
	first.ObserveMFASecurity(context.Background(), signal)
	second.ObserveMFASecurity(context.Background(), signal)
	shutdownTestHandler(t, first)
	shutdownTestHandler(t, second)

	firstItems := firstSender.snapshot()
	secondItems := secondSender.snapshot()
	if len(firstItems) != 1 || len(secondItems) != 1 {
		t.Fatalf("instance metric counts = %d and %d, want one each", len(firstItems), len(secondItems))
	}
	firstSeries := firstItems[0].metricPayload.Series[0]
	secondSeries := secondItems[0].metricPayload.Series[0]
	if firstSeries.Metric != secondSeries.Metric ||
		strings.Join(firstSeries.Tags, ",") != strings.Join(secondSeries.Tags, ",") {
		t.Fatalf("instances emitted different series groups: %v vs %v", firstSeries.Tags, secondSeries.Tags)
	}
	if got, want := firstSeries.Tags[2], "alert_reasons:failure_events-replay_events"; got != want {
		t.Fatalf("multi-reason tag = %q, want %q", got, want)
	}
}

func TestParseAlertEventRejectsUnexpectedEnvironmentAndReason(t *testing.T) {
	now := time.Now().UTC()
	record := slog.NewRecord(now, slog.LevelWarn, alertMessage, 0)
	record.AddAttrs(
		slog.Bool("alert", true),
		slog.Bool("recovery", false),
		slog.String("operation", alertOperation),
		slog.String("environment", "production"),
		slog.String("alert_reasons", "user_id"),
		slog.Int("window_minutes", 15),
	)
	if _, ok := parseAlertEvent(record, nil, "development"); ok {
		t.Fatal("event with a mismatched environment or user-controlled reason was accepted")
	}
}

func TestDatadogLiveIntakeSmoke(t *testing.T) {
	if os.Getenv("ASKOLO_DATADOG_INTAKE_SMOKE") != "1" {
		t.Skip("set ASKOLO_DATADOG_INTAKE_SMOKE=1 to send a Development alert and recovery sample")
	}
	apiKey := os.Getenv("DD_API_KEY")
	site := os.Getenv("DD_SITE")
	if apiKey == "" || !supportedSite(site) {
		t.Fatal("DD_API_KEY and a supported DD_SITE are required for the live intake smoke test")
	}

	now := time.Now().UTC()
	counts := map[string]int64{
		"failure_events":                               20,
		"replay_events":                                0,
		"lockout_events":                               0,
		"decryption_failure_events":                    0,
		"recovery_support_requests":                    0,
		"recovery_support_verification_failures":       0,
		"recovery_support_rate_limited":                0,
		"recovery_support_session_revocation_failures": 0,
		"affected_users":                               1,
	}
	transport := newAPITransport(Config{
		APIKey:      apiKey,
		Site:        site,
		Environment: "development",
	})
	alert := alertEvent{
		message:     alertMessage,
		environment: "development",
		reasonSet:   "failure_events",
		window:      15,
		alert:       true,
		counts:      counts,
		at:          now,
	}
	recovery := alert
	recovery.message = recoveryMessage
	recovery.alert = false
	recovery.recovery = true
	recovery.at = now.Add(time.Second)

	alertResult := sendLiveSample(transport, alert, 1)
	recoveryResult := sendLiveSample(transport, recovery, 0)
	for _, sample := range []struct {
		message string
		result  delivery
	}{
		{message: alert.message, result: alertResult},
		{message: recovery.message, result: recoveryResult},
	} {
		if sample.result.logFailed || sample.result.metricFailed {
			t.Errorf("Datadog intake rejected the %s sample (logs=%d metrics=%d)", sample.message, sample.result.logStatus, sample.result.metricStatus)
		}
		if sample.result.logStatus != http.StatusAccepted || sample.result.metricStatus != http.StatusAccepted {
			t.Errorf("Datadog intake statuses for %s = logs:%d metrics:%d, want 202 for both", sample.message, sample.result.logStatus, sample.result.metricStatus)
		}
	}
}

func sendLiveSample(transport *apiTransport, event alertEvent, value float64) delivery {
	return transport.send(context.Background(), submission{
		logItem:       buildLogItem(event),
		metricPayload: metricSubmission(event.environment, event.reasonSet, value, event.at).metricPayload,
		environment:   event.environment,
	})
}
