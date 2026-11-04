package datadogmfa

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/modules/auth"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

const (
	alertMessage          = "MFA verification failure spike"
	recoveryMessage       = "MFA verification failure spike recovered"
	alertOperation        = "mfa_security_spike"
	activeMetricName      = "askolo.mfa.security.alert_active"
	activeMetricHeartbeat = 30 * time.Second
	requestTimeout        = 3 * time.Second
	telemetryQueueSize    = 64
)

var allowedAlertReasons = map[string]struct{}{
	"failure_events":                               {},
	"replay_events":                                {},
	"lockout_events":                               {},
	"decryption_failure_events":                    {},
	"recovery_support_request_spike":               {},
	"recovery_support_verification_failures":       {},
	"recovery_support_rate_limited":                {},
	"recovery_support_session_revocation_failures": {},
}

var aggregateCountFields = []string{
	"failure_events",
	"replay_events",
	"lockout_events",
	"decryption_failure_events",
	"recovery_support_requests",
	"recovery_support_verification_failures",
	"recovery_support_rate_limited",
	"recovery_support_session_revocation_failures",
	"affected_users",
}

type Config struct {
	APIKey      string
	Site        string
	Environment string
}

type Handler struct {
	next       slog.Handler
	core       *handlerCore
	boundAttrs []slog.Attr
	grouped    bool
}

type handlerCore struct {
	config   Config
	sender   telemetrySender
	reporter slog.Handler
	queue    chan submission
	stop     chan struct{}
	done     chan struct{}

	lifecycleMu sync.Mutex
	closing     bool
	stateMu     sync.Mutex
	activeKey   string
	lastMetric  map[string]time.Time
	now         func() time.Time
}

type telemetrySender interface {
	send(context.Context, submission) delivery
}

type submission struct {
	logItem       *datadogV2.HTTPLogItem
	metricPayload *datadogV2.MetricPayload
	environment   string
}

type delivery struct {
	logStatus    int
	logFailed    bool
	metricStatus int
	metricFailed bool
}

type alertEvent struct {
	message     string
	environment string
	reasonSet   string
	window      int64
	alert       bool
	recovery    bool
	counts      map[string]int64
	at          time.Time
}

func New(next slog.Handler, cfg Config) (*Handler, func(context.Context) error, error) {
	if next == nil {
		return nil, nil, fmt.Errorf("Datadog MFA telemetry requires a log handler")
	}
	handler := &Handler{next: next}
	if cfg.APIKey == "" {
		return handler, func(context.Context) error { return nil }, nil
	}
	if cfg.Site == "" || !supportedSite(cfg.Site) || !supportedEnvironment(cfg.Environment) {
		return nil, nil, fmt.Errorf("invalid Datadog MFA telemetry configuration")
	}

	sender := newAPITransport(cfg)
	core := &handlerCore{
		config:     cfg,
		sender:     sender,
		reporter:   next,
		queue:      make(chan submission, telemetryQueueSize),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		lastMetric: make(map[string]time.Time),
		now:        time.Now,
	}
	handler.core = core
	go core.run()
	return handler, core.shutdown, nil
}

func newAPITransport(cfg Config) *apiTransport {
	apiConfiguration := datadog.NewConfiguration()
	apiConfiguration.HTTPClient = &http.Client{Timeout: requestTimeout}
	apiConfiguration.Compress = false
	apiConfiguration.RetryConfiguration.EnableRetry = false
	apiClient := datadog.NewAPIClient(apiConfiguration)
	return &apiTransport{
		apiKey:  cfg.APIKey,
		site:    cfg.Site,
		logs:    datadogV2.NewLogsApi(apiClient),
		metrics: datadogV2.NewMetricsApi(apiClient),
	}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if err := h.next.Handle(ctx, record); err != nil {
		return err
	}
	if h.core == nil || h.grouped {
		return nil
	}
	event, ok := parseAlertEvent(record, h.boundAttrs, h.core.config.Environment)
	if !ok {
		return nil
	}
	h.core.enqueue(submission{
		logItem:     buildLogItem(event),
		environment: event.environment,
	})
	return nil
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	bound := append([]slog.Attr(nil), h.boundAttrs...)
	bound = append(bound, attrs...)
	return &Handler{
		next:       h.next.WithAttrs(attrs),
		core:       h.core,
		boundAttrs: bound,
		grouped:    h.grouped,
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		next:       h.next.WithGroup(name),
		core:       h.core,
		boundAttrs: h.boundAttrs,
		grouped:    true,
	}
}

func (h *Handler) ObserveMFASecurity(_ context.Context, signal auth.MFASecurityReadiness) {
	if h.core != nil {
		h.core.observe(signal)
	}
}

func (c *handlerCore) observe(signal auth.MFASecurityReadiness) {
	if signal.Status != "available" ||
		signal.Environment != c.config.Environment ||
		signal.WindowMinutes != 15 {
		return
	}

	reasonSet := strings.Join(signal.AlertReasons, ",")
	if signal.Alert && !validReasonSet(reasonSet) {
		return
	}
	now := c.now().UTC()
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if signal.Alert {
		if c.activeKey != "" && c.activeKey != reasonSet {
			c.enqueue(metricSubmission(c.config.Environment, c.activeKey, 0, now))
			delete(c.lastMetric, c.activeKey)
		}
		lastSent := c.lastMetric[reasonSet]
		if c.activeKey != reasonSet || lastSent.IsZero() || now.Sub(lastSent) >= activeMetricHeartbeat {
			c.enqueue(metricSubmission(c.config.Environment, reasonSet, 1, now))
			c.lastMetric[reasonSet] = now
		}
		c.activeKey = reasonSet
		return
	}

	if reasonSet == "" {
		reasonSet = c.activeKey
	}
	if !validReasonSet(reasonSet) {
		return
	}
	if c.activeKey == "" || c.activeKey == reasonSet {
		c.enqueue(metricSubmission(c.config.Environment, reasonSet, 0, now))
		delete(c.lastMetric, reasonSet)
		if c.activeKey == reasonSet {
			c.activeKey = ""
		}
	}
}

func (c *handlerCore) enqueue(item submission) {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.closing {
		return
	}
	select {
	case c.queue <- item:
	default:
		c.report(item.environment, "queue", 0)
	}
}

func (c *handlerCore) run() {
	defer close(c.done)
	for {
		select {
		case item := <-c.queue:
			c.deliver(item)
		case <-c.stop:
			for {
				select {
				case item := <-c.queue:
					c.deliver(item)
				default:
					return
				}
			}
		}
	}
}

func (c *handlerCore) deliver(item submission) {
	result := c.sender.send(context.Background(), item)
	if result.logFailed {
		c.report(item.environment, "logs", result.logStatus)
	}
	if result.metricFailed {
		c.report(item.environment, "metrics", result.metricStatus)
	}
}

func (c *handlerCore) report(environment, operation string, status int) {
	record := slog.NewRecord(time.Now().UTC(), slog.LevelWarn, "Datadog MFA telemetry delivery failed", 0)
	attrs := []slog.Attr{
		slog.String("environment", environment),
		slog.String("operation", "mfa_telemetry_"+operation),
	}
	if status > 0 {
		attrs = append(attrs, slog.Int("http_status", status))
	}
	record.AddAttrs(attrs...)
	_ = c.reporter.Handle(context.Background(), record)
}

func (c *handlerCore) shutdown(ctx context.Context) error {
	c.lifecycleMu.Lock()
	if !c.closing {
		c.closing = true
		close(c.stop)
	}
	c.lifecycleMu.Unlock()

	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func parseAlertEvent(record slog.Record, bound []slog.Attr, expectedEnvironment string) (alertEvent, bool) {
	isAlert := record.Message == alertMessage
	isRecovery := record.Message == recoveryMessage
	if !isAlert && !isRecovery {
		return alertEvent{}, false
	}

	values := make(map[string]slog.Value, len(bound)+16)
	add := func(attr slog.Attr) {
		if attr.Value.Kind() == slog.KindGroup {
			return
		}
		values[attr.Key] = attr.Value.Resolve()
	}
	for _, attr := range bound {
		add(attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		add(attr)
		return true
	})

	environment, ok := stringAttr(values, "environment")
	if !ok || environment != expectedEnvironment || !supportedEnvironment(environment) {
		return alertEvent{}, false
	}
	operation, ok := stringAttr(values, "operation")
	if !ok || operation != alertOperation {
		return alertEvent{}, false
	}
	reasonSet, ok := stringAttr(values, "alert_reasons")
	if !ok || !validReasonSet(reasonSet) {
		return alertEvent{}, false
	}
	window, ok := intAttr(values, "window_minutes")
	if !ok || window != 15 {
		return alertEvent{}, false
	}
	alertValue, hasAlert := boolAttr(values, "alert")
	recoveryValue, hasRecovery := boolAttr(values, "recovery")
	if !hasAlert || !hasRecovery ||
		(isAlert && (!alertValue || recoveryValue)) ||
		(isRecovery && (alertValue || !recoveryValue)) {
		return alertEvent{}, false
	}

	counts := make(map[string]int64, len(aggregateCountFields))
	for _, field := range aggregateCountFields {
		count, ok := intAttr(values, field)
		if !ok || count < 0 {
			return alertEvent{}, false
		}
		counts[field] = count
	}

	at := record.Time
	if at.IsZero() {
		at = time.Now()
	}
	return alertEvent{
		message:     record.Message,
		environment: environment,
		reasonSet:   reasonSet,
		window:      window,
		alert:       isAlert,
		recovery:    isRecovery,
		counts:      counts,
		at:          at,
	}, true
}

func stringAttr(values map[string]slog.Value, key string) (string, bool) {
	value, ok := values[key]
	if !ok || value.Kind() != slog.KindString {
		return "", false
	}
	return value.String(), true
}

func intAttr(values map[string]slog.Value, key string) (int64, bool) {
	value, ok := values[key]
	if !ok || value.Kind() != slog.KindInt64 {
		return 0, false
	}
	return value.Int64(), true
}

func boolAttr(values map[string]slog.Value, key string) (bool, bool) {
	value, ok := values[key]
	if !ok || value.Kind() != slog.KindBool {
		return false, false
	}
	return value.Bool(), true
}

func buildLogItem(event alertEvent) *datadogV2.HTTPLogItem {
	attributes := map[string]interface{}{
		"environment":                                  event.environment,
		"operation":                                    alertOperation,
		"alert_reasons":                                event.reasonSet,
		"failure_events":                               event.counts["failure_events"],
		"replay_events":                                event.counts["replay_events"],
		"lockout_events":                               event.counts["lockout_events"],
		"decryption_failure_events":                    event.counts["decryption_failure_events"],
		"recovery_support_requests":                    event.counts["recovery_support_requests"],
		"recovery_support_verification_failures":       event.counts["recovery_support_verification_failures"],
		"recovery_support_rate_limited":                event.counts["recovery_support_rate_limited"],
		"recovery_support_session_revocation_failures": event.counts["recovery_support_session_revocation_failures"],
		"affected_users":                               event.counts["affected_users"],
	}
	return &datadogV2.HTTPLogItem{
		Message:              event.message,
		AdditionalProperties: attributes,
	}
}

func metricSubmission(environment, reasonSet string, value float64, at time.Time) submission {
	metricType := datadogV2.METRICINTAKETYPE_GAUGE
	series := datadogV2.MetricSeries{
		Metric: activeMetricName,
		Type:   &metricType,
		Points: []datadogV2.MetricPoint{{
			Timestamp: datadog.PtrInt64(at.UTC().Unix()),
			Value:     datadog.PtrFloat64(value),
		}},
		Tags: []string{
			"environment:" + environment,
			"operation:" + alertOperation,
			"alert_reasons:" + strings.ReplaceAll(reasonSet, ",", "-"),
		},
	}
	payload := datadogV2.MetricPayload{Series: []datadogV2.MetricSeries{series}}
	return submission{
		metricPayload: &payload,
		environment:   environment,
	}
}

type apiTransport struct {
	apiKey  string
	site    string
	logs    *datadogV2.LogsApi
	metrics *datadogV2.MetricsApi
}

func (s *apiTransport) send(ctx context.Context, item submission) delivery {
	result := delivery{}
	if item.logItem != nil {
		requestContext, cancel := s.requestContext(ctx)
		_, response, err := s.logs.SubmitLog(requestContext, []datadogV2.HTTPLogItem{*item.logItem})
		cancel()
		if response != nil {
			result.logStatus = response.StatusCode
		}
		result.logFailed = err != nil
	}
	if item.metricPayload != nil {
		requestContext, cancel := s.requestContext(ctx)
		_, response, err := s.metrics.SubmitMetrics(requestContext, *item.metricPayload)
		cancel()
		if response != nil {
			result.metricStatus = response.StatusCode
		}
		result.metricFailed = err != nil
	}
	return result
}

func (s *apiTransport) requestContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	ctx = context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{"site": s.site})
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: s.apiKey},
	})
	return ctx, cancel
}

func supportedSite(site string) bool {
	switch site {
	case "datadoghq.com", "us3.datadoghq.com", "us5.datadoghq.com",
		"ap1.datadoghq.com", "ap2.datadoghq.com", "uk1.datadoghq.com",
		"datadoghq.eu", "ddog-gov.com", "us2.ddog-gov.com":
		return true
	default:
		return false
	}
}

func supportedEnvironment(environment string) bool {
	switch environment {
	case "development", "staging", "production":
		return true
	default:
		return false
	}
}

func validReasonSet(value string) bool {
	if value == "" {
		return false
	}
	seen := make(map[string]struct{})
	for _, reason := range strings.Split(value, ",") {
		if _, ok := allowedAlertReasons[reason]; !ok {
			return false
		}
		if _, exists := seen[reason]; exists {
			return false
		}
		seen[reason] = struct{}{}
	}
	return true
}
