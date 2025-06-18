package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// PrometheusMonitor implements monitoring using Prometheus
type PrometheusMonitor struct {
	config   *PrometheusConfig
	registry *prometheus.Registry
	logger   *zap.SugaredLogger
	
	// Standard metrics
	requestsTotal       *prometheus.CounterVec
	requestDuration     *prometheus.HistogramVec
	tokensTotal         *prometheus.CounterVec
	costTotal           *prometheus.CounterVec
	errorsTotal         *prometheus.CounterVec
	cacheHitsTotal      *prometheus.CounterVec
	providerLatency     *prometheus.HistogramVec
	activeConnections   prometheus.Gauge
	queueSize          prometheus.Gauge
	
	// Custom metrics map
	customMetrics map[string]prometheus.Collector
}

// NewPrometheusMonitor creates a new Prometheus monitor
func NewPrometheusMonitor(config *PrometheusConfig, logger *zap.SugaredLogger) (*PrometheusMonitor, error) {
	registry := prometheus.NewRegistry()
	
	pm := &PrometheusMonitor{
		config:        config,
		registry:      registry,
		logger:        logger,
		customMetrics: make(map[string]prometheus.Collector),
	}
	
	// Initialize standard metrics
	if err := pm.initializeMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %v", err)
	}
	
	return pm, nil
}

// initializeMetrics initializes all Prometheus metrics
func (p *PrometheusMonitor) initializeMetrics() error {
	namespace := p.config.Namespace
	subsystem := p.config.Subsystem
	
	// Request metrics
	p.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Total number of requests",
		},
		[]string{"provider", "model", "endpoint", "method", "status_code", "user_id", "team_id"},
	)
	
	p.requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"provider", "model", "endpoint", "method"},
	)
	
	// Token metrics
	p.tokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "tokens_total",
			Help:      "Total number of tokens processed",
		},
		[]string{"provider", "model", "type", "user_id", "team_id"}, // type: input, output, total
	)
	
	// Cost metrics
	p.costTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cost_total",
			Help:      "Total cost of requests",
		},
		[]string{"provider", "model", "user_id", "team_id"},
	)
	
	// Error metrics
	p.errorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors",
		},
		[]string{"provider", "model", "endpoint", "error_type"},
	)
	
	// Cache metrics
	p.cacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "cache_hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{"cache_type", "hit"}, // hit: true, false
	)
	
	// Provider latency
	p.providerLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "provider_latency_seconds",
			Help:      "Provider response latency in seconds",
			Buckets:   []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0},
		},
		[]string{"provider", "model"},
	)
	
	// System metrics
	p.activeConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "active_connections",
			Help:      "Number of active connections",
		},
	)
	
	p.queueSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_size",
			Help:      "Current queue size",
		},
	)
	
	// Register all metrics
	collectors := []prometheus.Collector{
		p.requestsTotal,
		p.requestDuration,
		p.tokensTotal,
		p.costTotal,
		p.errorsTotal,
		p.cacheHitsTotal,
		p.providerLatency,
		p.activeConnections,
		p.queueSize,
	}
	
	for _, collector := range collectors {
		if err := p.registry.Register(collector); err != nil {
			return fmt.Errorf("failed to register metric: %v", err)
		}
	}
	
	return nil
}

// RecordMetric records a custom metric
func (p *PrometheusMonitor) RecordMetric(metric *Metric) error {
	switch metric.Type {
	case MetricTypeCounter:
		return p.recordCounter(metric)
	case MetricTypeGauge:
		return p.recordGauge(metric)
	case MetricTypeHistogram:
		return p.recordHistogram(metric)
	case MetricTypeSummary:
		return p.recordSummary(metric)
	default:
		return fmt.Errorf("unsupported metric type: %s", metric.Type)
	}
}

// recordCounter records a counter metric
func (p *PrometheusMonitor) recordCounter(metric *Metric) error {
	// Check if custom counter exists
	if collector, exists := p.customMetrics[metric.Name]; exists {
		if counter, ok := collector.(*prometheus.CounterVec); ok {
			counter.With(metric.Labels).Add(metric.Value)
			return nil
		}
	}
	
	// Create new counter if it doesn't exist
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: p.config.Namespace,
			Subsystem: p.config.Subsystem,
			Name:      metric.Name,
			Help:      fmt.Sprintf("Custom counter metric: %s", metric.Name),
		},
		getLabelNames(metric.Labels),
	)
	
	if err := p.registry.Register(counter); err != nil {
		return fmt.Errorf("failed to register counter %s: %v", metric.Name, err)
	}
	
	p.customMetrics[metric.Name] = counter
	counter.With(metric.Labels).Add(metric.Value)
	
	return nil
}

// recordGauge records a gauge metric
func (p *PrometheusMonitor) recordGauge(metric *Metric) error {
	// Check if custom gauge exists
	if collector, exists := p.customMetrics[metric.Name]; exists {
		if gauge, ok := collector.(*prometheus.GaugeVec); ok {
			gauge.With(metric.Labels).Set(metric.Value)
			return nil
		}
	}
	
	// Create new gauge if it doesn't exist
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: p.config.Namespace,
			Subsystem: p.config.Subsystem,
			Name:      metric.Name,
			Help:      fmt.Sprintf("Custom gauge metric: %s", metric.Name),
		},
		getLabelNames(metric.Labels),
	)
	
	if err := p.registry.Register(gauge); err != nil {
		return fmt.Errorf("failed to register gauge %s: %v", metric.Name, err)
	}
	
	p.customMetrics[metric.Name] = gauge
	gauge.With(metric.Labels).Set(metric.Value)
	
	return nil
}

// recordHistogram records a histogram metric
func (p *PrometheusMonitor) recordHistogram(metric *Metric) error {
	// Check if custom histogram exists
	if collector, exists := p.customMetrics[metric.Name]; exists {
		if histogram, ok := collector.(*prometheus.HistogramVec); ok {
			histogram.With(metric.Labels).Observe(metric.Value)
			return nil
		}
	}
	
	// Create new histogram if it doesn't exist
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: p.config.Namespace,
			Subsystem: p.config.Subsystem,
			Name:      metric.Name,
			Help:      fmt.Sprintf("Custom histogram metric: %s", metric.Name),
			Buckets:   prometheus.DefBuckets,
		},
		getLabelNames(metric.Labels),
	)
	
	if err := p.registry.Register(histogram); err != nil {
		return fmt.Errorf("failed to register histogram %s: %v", metric.Name, err)
	}
	
	p.customMetrics[metric.Name] = histogram
	histogram.With(metric.Labels).Observe(metric.Value)
	
	return nil
}

// recordSummary records a summary metric
func (p *PrometheusMonitor) recordSummary(metric *Metric) error {
	// Check if custom summary exists
	if collector, exists := p.customMetrics[metric.Name]; exists {
		if summary, ok := collector.(*prometheus.SummaryVec); ok {
			summary.With(metric.Labels).Observe(metric.Value)
			return nil
		}
	}
	
	// Create new summary if it doesn't exist
	summary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: p.config.Namespace,
			Subsystem: p.config.Subsystem,
			Name:      metric.Name,
			Help:      fmt.Sprintf("Custom summary metric: %s", metric.Name),
		},
		getLabelNames(metric.Labels),
	)
	
	if err := p.registry.Register(summary); err != nil {
		return fmt.Errorf("failed to register summary %s: %v", metric.Name, err)
	}
	
	p.customMetrics[metric.Name] = summary
	summary.With(metric.Labels).Observe(metric.Value)
	
	return nil
}

// RecordRequestMetrics records request-specific metrics
func (p *PrometheusMonitor) RecordRequestMetrics(metrics *RequestMetrics) error {
	labels := prometheus.Labels{
		"provider":    metrics.Provider,
		"model":       metrics.Model,
		"endpoint":    metrics.Endpoint,
		"method":      metrics.Method,
		"status_code": strconv.Itoa(metrics.StatusCode),
		"user_id":     metrics.UserID,
		"team_id":     metrics.TeamID,
	}
	
	// Record request count
	p.requestsTotal.With(labels).Inc()
	
	// Record request duration
	durationLabels := prometheus.Labels{
		"provider": metrics.Provider,
		"model":    metrics.Model,
		"endpoint": metrics.Endpoint,
		"method":   metrics.Method,
	}
	p.requestDuration.With(durationLabels).Observe(metrics.Duration.Seconds())
	
	// Record token metrics
	tokenLabels := prometheus.Labels{
		"provider": metrics.Provider,
		"model":    metrics.Model,
		"user_id":  metrics.UserID,
		"team_id":  metrics.TeamID,
	}
	
	if metrics.InputTokens > 0 {
		inputLabels := make(prometheus.Labels)
		for k, v := range tokenLabels {
			inputLabels[k] = v
		}
		inputLabels["type"] = "input"
		p.tokensTotal.With(inputLabels).Add(float64(metrics.InputTokens))
	}
	
	if metrics.OutputTokens > 0 {
		outputLabels := make(prometheus.Labels)
		for k, v := range tokenLabels {
			outputLabels[k] = v
		}
		outputLabels["type"] = "output"
		p.tokensTotal.With(outputLabels).Add(float64(metrics.OutputTokens))
	}
	
	if metrics.TotalTokens > 0 {
		totalLabels := make(prometheus.Labels)
		for k, v := range tokenLabels {
			totalLabels[k] = v
		}
		totalLabels["type"] = "total"
		p.tokensTotal.With(totalLabels).Add(float64(metrics.TotalTokens))
	}
	
	// Record cost
	if metrics.Cost > 0 {
		p.costTotal.With(tokenLabels).Add(metrics.Cost)
	}
	
	// Record cache hit
	cacheLabels := prometheus.Labels{
		"cache_type": "response",
		"hit":        strconv.FormatBool(metrics.CacheHit),
	}
	p.cacheHitsTotal.With(cacheLabels).Inc()
	
	// Record provider latency
	if metrics.Provider != "" {
		providerLabels := prometheus.Labels{
			"provider": metrics.Provider,
			"model":    metrics.Model,
		}
		p.providerLatency.With(providerLabels).Observe(metrics.Duration.Seconds())
	}
	
	return nil
}

// RecordError records error metrics
func (p *PrometheusMonitor) RecordError(errorMsg string, labels map[string]string) error {
	errorLabels := prometheus.Labels{
		"provider":   getLabel(labels, "provider"),
		"model":      getLabel(labels, "model"),
		"endpoint":   getLabel(labels, "endpoint"),
		"error_type": errorMsg,
	}
	
	p.errorsTotal.With(errorLabels).Inc()
	return nil
}

// StartRequest starts request tracing (no-op for Prometheus)
func (p *PrometheusMonitor) StartRequest(ctx context.Context, operation string) (context.Context, func()) {
	start := time.Now()
	
	return ctx, func() {
		// Could record operation duration here if needed
		p.logger.Debugw("Request completed", "operation", operation, "duration", time.Since(start))
	}
}

// GetHandler returns the Prometheus HTTP handler
func (p *PrometheusMonitor) GetHandler() http.Handler {
	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
}

// UpdateSystemMetrics updates system-level metrics
func (p *PrometheusMonitor) UpdateSystemMetrics(activeConns int, queueSize int) {
	p.activeConnections.Set(float64(activeConns))
	p.queueSize.Set(float64(queueSize))
}

// Flush flushes metrics (no-op for Prometheus)
func (p *PrometheusMonitor) Flush() error {
	return nil
}

// Close closes the monitor (no-op for Prometheus)
func (p *PrometheusMonitor) Close() error {
	return nil
}

// Helper functions

// getLabelNames extracts label names from a labels map
func getLabelNames(labels map[string]string) []string {
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	return names
}

// getLabel safely gets a label value
func getLabel(labels map[string]string, key string) string {
	if labels == nil {
		return ""
	}
	return labels[key]
}