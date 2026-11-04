package datadogmetrics

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

const (
	requestDurationMetric = "askolo.http.server.duration_ms"
	flushInterval         = 30 * time.Second
	requestTimeout        = 3 * time.Second
	maxMetricGroups       = 256
	maxSamplesPerGroup    = 1024
)

type Config struct {
	APIKey                string
	Site                  string
	Environment           string
	Service               string
	ReadDatabasePoolStats func() DatabasePoolSnapshot
}

type DatabasePoolSnapshot struct {
	TotalConnections    float64
	AcquiredConnections float64
	IdleConnections     float64
	MaxConnections      float64
	AcquireCount        float64
	AcquireDurationMS   float64
}

type RequestObserver interface {
	ObserveHTTPRequest(method, path string, status int, duration time.Duration)
}

type distributionSender interface {
	submit(context.Context, metricPayload) (int, error)
}

type metricPayload struct {
	distributions datadogV1.DistributionPointsPayload
	counts        datadogV2.MetricPayload
}

type groupKey struct {
	routeGroup  string
	method      string
	statusClass string
}

type aggregate struct {
	count     uint64
	durations []float64
}

type Handler struct {
	config Config
	logger *slog.Logger
	sender distributionSender
	random *rand.Rand

	mu      sync.Mutex
	groups  map[groupKey]*aggregate
	closing bool
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func New(cfg Config, logger *slog.Logger) (*Handler, func(context.Context) error, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.APIKey == "" {
		return nil, func(context.Context) error { return nil }, nil
	}
	if !supportedSite(cfg.Site) || !supportedEnvironment(cfg.Environment) || !safeService(cfg.Service) {
		return nil, nil, fmt.Errorf("invalid Datadog HTTP metrics configuration")
	}

	apiConfiguration := datadog.NewConfiguration()
	apiConfiguration.HTTPClient = &http.Client{Timeout: requestTimeout}
	apiConfiguration.Compress = false
	apiConfiguration.RetryConfiguration.EnableRetry = false
	client := datadog.NewAPIClient(apiConfiguration)
	sender := &apiSender{
		site:      cfg.Site,
		apiKey:    cfg.APIKey,
		metricsV1: datadogV1.NewMetricsApi(client),
		metricsV2: datadogV2.NewMetricsApi(client),
	}

	handler := newHandler(cfg, logger, sender)
	go handler.run()
	return handler, handler.shutdown, nil
}

func newHandler(cfg Config, logger *slog.Logger, sender distributionSender) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		config: cfg,
		logger: logger,
		sender: sender,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
		groups: make(map[groupKey]*aggregate),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (h *Handler) ObserveHTTPRequest(method, path string, status int, duration time.Duration) {
	if h == nil {
		return
	}
	key := groupKey{
		routeGroup:  routeGroup(path),
		method:      methodLabel(method),
		statusClass: statusClass(status),
	}
	value := float64(duration) / float64(time.Millisecond)
	if value < 0 {
		value = 0
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closing {
		return
	}
	bucket := h.groups[key]
	if bucket == nil {
		if len(h.groups) >= maxMetricGroups {
			return
		}
		bucket = &aggregate{durations: make([]float64, 0, maxSamplesPerGroup)}
		h.groups[key] = bucket
	}
	bucket.count++
	if len(bucket.durations) < maxSamplesPerGroup {
		bucket.durations = append(bucket.durations, value)
		return
	}

	// Keep a bounded reservoir while preserving an unbiased latency sample.
	if position := h.random.Int63n(int64(bucket.count)); position < maxSamplesPerGroup {
		bucket.durations[position] = value
	}
}

func (h *Handler) run() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	defer close(h.done)

	for {
		select {
		case <-ticker.C:
			h.flush()
		case <-h.stop:
			h.flush()
			return
		}
	}
}

func (h *Handler) flush() {
	batch := h.take()
	payload := buildPayload(h.config, batch, time.Now().UTC())
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	status, err := h.sender.submit(ctx, payload)
	cancel()
	if err != nil {
		h.report(status)
	}
}

func (h *Handler) take() map[groupKey]aggregate {
	h.mu.Lock()
	defer h.mu.Unlock()
	batch := h.groups
	h.groups = make(map[groupKey]*aggregate)
	result := make(map[groupKey]aggregate, len(batch))
	for key, value := range batch {
		result[key] = aggregate{
			count:     value.count,
			durations: append([]float64(nil), value.durations...),
		}
	}
	return result
}

func (h *Handler) report(status int) {
	attrs := []any{
		"environment", h.config.Environment,
		"operation", "http_request_metrics",
	}
	if status > 0 {
		attrs = append(attrs, "http_status", status)
	}
	h.logger.Warn("Datadog HTTP metrics delivery failed", attrs...)
}

func (h *Handler) shutdown(ctx context.Context) error {
	h.mu.Lock()
	if !h.closing {
		h.closing = true
		h.once.Do(func() { close(h.stop) })
	}
	h.mu.Unlock()

	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func buildPayload(cfg Config, values map[groupKey]aggregate, at time.Time) metricPayload {
	timestamp := float64(at.UTC().Unix())
	pointType := datadogV1.DISTRIBUTIONPOINTSTYPE_DISTRIBUTION
	distributionSeries := make([]datadogV1.DistributionPointsSeries, 0, len(values))
	countSeries := make([]datadogV2.MetricSeries, 0, len(values)+12)
	for key, value := range values {
		if value.count == 0 || len(value.durations) == 0 {
			continue
		}
		samples := append([]float64(nil), value.durations...)
		tags := []string{
			"environment:" + cfg.Environment,
			"service:" + cfg.Service,
			"route_group:" + key.routeGroup,
			"method:" + key.method,
			"status_class:" + key.statusClass,
		}
		distributionSeries = append(distributionSeries, datadogV1.DistributionPointsSeries{
			Metric: requestDurationMetric,
			Type:   &pointType,
			Points: [][]datadogV1.DistributionPointItem{{
				datadogV1.DistributionPointTimestampAsDistributionPointItem(&timestamp),
				datadogV1.DistributionPointDataAsDistributionPointItem(&samples),
			}},
			Tags: tags,
		})
		metricType := datadogV2.METRICINTAKETYPE_GAUGE
		countSeries = append(countSeries, datadogV2.MetricSeries{
			Metric: "askolo.http.server.request_count_30s",
			Type:   &metricType,
			Points: []datadogV2.MetricPoint{{
				Timestamp: datadog.PtrInt64(at.UTC().Unix()),
				Value:     datadog.PtrFloat64(float64(value.count)),
			}},
			Tags: tags,
		})
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	baseTags := []string{
		"environment:" + cfg.Environment,
		"service:" + cfg.Service,
	}
	countSeries = append(countSeries,
		gaugeSeries("askolo.go.goroutines", float64(runtime.NumGoroutine()), baseTags, at),
		gaugeSeries("askolo.go.heap_alloc_bytes", float64(memory.HeapAlloc), baseTags, at),
		gaugeSeries("askolo.go.gc_pause_ms_total", float64(memory.PauseTotalNs)/float64(time.Millisecond), baseTags, at),
	)
	if cfg.ReadDatabasePoolStats != nil {
		stats := cfg.ReadDatabasePoolStats()
		countSeries = append(countSeries,
			gaugeSeries("askolo.db.pool.total_connections", stats.TotalConnections, baseTags, at),
			gaugeSeries("askolo.db.pool.acquired_connections", stats.AcquiredConnections, baseTags, at),
			gaugeSeries("askolo.db.pool.idle_connections", stats.IdleConnections, baseTags, at),
			gaugeSeries("askolo.db.pool.max_connections", stats.MaxConnections, baseTags, at),
			gaugeSeries("askolo.db.pool.acquire_count_total", stats.AcquireCount, baseTags, at),
			gaugeSeries("askolo.db.pool.acquire_duration_ms_total", stats.AcquireDurationMS, baseTags, at),
		)
	}
	return metricPayload{
		distributions: datadogV1.DistributionPointsPayload{Series: distributionSeries},
		counts:        datadogV2.MetricPayload{Series: countSeries},
	}
}

func gaugeSeries(name string, value float64, tags []string, at time.Time) datadogV2.MetricSeries {
	metricType := datadogV2.METRICINTAKETYPE_GAUGE
	return datadogV2.MetricSeries{
		Metric: name,
		Type:   &metricType,
		Points: []datadogV2.MetricPoint{{
			Timestamp: datadog.PtrInt64(at.UTC().Unix()),
			Value:     datadog.PtrFloat64(value),
		}},
		Tags: append([]string(nil), tags...),
	}
}

func routeGroup(path string) string {
	switch {
	case path == "/" || path == "":
		return "root"
	case path == "/healthz" || path == "/readyz" || path == "/api/healthz":
		return "health"
	case strings.HasPrefix(path, "/api/v2/"):
		return "api_v2"
	case strings.HasPrefix(path, "/api/"):
		return "api"
	default:
		return "web"
	}
}

func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func statusClass(status int) string {
	switch {
	case status >= 100 && status < 200:
		return "1xx"
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return "unknown"
	}
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

func safeService(service string) bool {
	if len(service) == 0 || len(service) > 64 || strings.TrimSpace(service) != service {
		return false
	}
	for _, r := range service {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

type apiSender struct {
	site      string
	apiKey    string
	metricsV1 *datadogV1.MetricsApi
	metricsV2 *datadogV2.MetricsApi
}

func (s *apiSender) submit(parent context.Context, payload metricPayload) (int, error) {
	ctx := context.WithValue(parent, datadog.ContextServerVariables, map[string]string{"site": s.site})
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: s.apiKey},
	})
	var status int
	if len(payload.distributions.Series) > 0 {
		_, response, err := s.metricsV1.SubmitDistributionPoints(ctx, payload.distributions)
		if response != nil {
			status = response.StatusCode
		}
		if err != nil {
			return status, err
		}
	}
	if len(payload.counts.Series) > 0 {
		_, response, err := s.metricsV2.SubmitMetrics(ctx, payload.counts)
		if response != nil {
			status = response.StatusCode
		}
		if err != nil {
			return status, err
		}
	}
	if status == 0 {
		return 0, fmt.Errorf("empty Datadog HTTP metrics payload")
	}
	return status, nil
}
