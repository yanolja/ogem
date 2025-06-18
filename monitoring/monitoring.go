package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// MetricType represents different types of metrics
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// MonitoringConfig represents monitoring configuration
type MonitoringConfig struct {
	// Enable monitoring
	Enabled bool `yaml:"enabled"`
	
	// Prometheus configuration
	Prometheus *PrometheusConfig `yaml:"prometheus,omitempty"`
	
	// Datadog configuration
	Datadog *DatadogConfig `yaml:"datadog,omitempty"`
	
	// OpenTelemetry configuration
	OpenTelemetry *OpenTelemetryConfig `yaml:"opentelemetry,omitempty"`
	
	// Custom metrics endpoint
	CustomEndpoint *CustomEndpointConfig `yaml:"custom_endpoint,omitempty"`
	
	// Sampling rate for traces (0.0 to 1.0)
	TracingSampleRate float64 `yaml:"tracing_sample_rate"`
	
	// Enable detailed model performance metrics
	DetailedMetrics bool `yaml:"detailed_metrics"`
}

// PrometheusConfig represents Prometheus configuration
type PrometheusConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Port       int    `yaml:"port"`
	Path       string `yaml:"path"`
	Namespace  string `yaml:"namespace"`
	Subsystem  string `yaml:"subsystem"`
}

// DatadogConfig represents Datadog configuration
type DatadogConfig struct {
	Enabled bool   `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
	AppKey  string `yaml:"app_key,omitempty"`
	Site    string `yaml:"site"` // e.g., "datadoghq.com", "datadoghq.eu"
	Service string `yaml:"service"`
	Env     string `yaml:"env"`
	Version string `yaml:"version"`
	Tags    []string `yaml:"tags,omitempty"`
}

// OpenTelemetryConfig represents OpenTelemetry configuration
type OpenTelemetryConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Endpoint     string `yaml:"endpoint"`
	ServiceName  string `yaml:"service_name"`
	ServiceVersion string `yaml:"service_version"`
	Environment  string `yaml:"environment"`
	Headers      map[string]string `yaml:"headers,omitempty"`
	Insecure     bool   `yaml:"insecure"`
}

// CustomEndpointConfig represents custom metrics endpoint configuration
type CustomEndpointConfig struct {
	Enabled bool              `yaml:"enabled"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Format  string            `yaml:"format"` // "json", "influxdb", "statsd"
}

// Metric represents a metric with labels
type Metric struct {
	Name      string
	Type      MetricType
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
}

// RequestMetrics represents metrics for a single request
type RequestMetrics struct {
	Provider     string
	Model        string
	Endpoint     string
	Method       string
	StatusCode   int
	Duration     time.Duration
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	Cost         float64
	UserID       string
	TeamID       string
	CacheHit     bool
	Error        string
}

// MonitoringManager handles all monitoring integrations
type MonitoringManager struct {
	config     *MonitoringConfig
	prometheus *PrometheusMonitor
	datadog    *DatadogMonitor
	otel       *OpenTelemetryMonitor
	custom     *CustomMonitor
	logger     *zap.SugaredLogger
}

// Monitor interface for different monitoring backends
type Monitor interface {
	RecordMetric(metric *Metric) error
	RecordRequestMetrics(metrics *RequestMetrics) error
	RecordError(errorMsg string, labels map[string]string) error
	StartRequest(ctx context.Context, operation string) (context.Context, func()) // Returns context and finish function
	Flush() error
	Close() error
}

// NewMonitoringManager creates a new monitoring manager
func NewMonitoringManager(config *MonitoringConfig, logger *zap.SugaredLogger) (*MonitoringManager, error) {
	if !config.Enabled {
		return &MonitoringManager{
			config: config,
			logger: logger,
		}, nil
	}
	
	manager := &MonitoringManager{
		config: config,
		logger: logger,
	}
	
	// Initialize Prometheus if enabled
	if config.Prometheus != nil && config.Prometheus.Enabled {
		prometheus, err := NewPrometheusMonitor(config.Prometheus, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Prometheus monitor: %v", err)
		}
		manager.prometheus = prometheus
	}
	
	// Initialize Datadog if enabled
	if config.Datadog != nil && config.Datadog.Enabled {
		datadog, err := NewDatadogMonitor(config.Datadog, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Datadog monitor: %v", err)
		}
		manager.datadog = datadog
	}
	
	// Initialize OpenTelemetry if enabled
	if config.OpenTelemetry != nil && config.OpenTelemetry.Enabled {
		otel, err := NewOpenTelemetryMonitor(config.OpenTelemetry, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OpenTelemetry monitor: %v", err)
		}
		manager.otel = otel
	}
	
	// Initialize Custom endpoint if enabled
	if config.CustomEndpoint != nil && config.CustomEndpoint.Enabled {
		custom, err := NewCustomMonitor(config.CustomEndpoint, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize custom monitor: %v", err)
		}
		manager.custom = custom
	}
	
	return manager, nil
}

// RecordMetric records a metric across all enabled monitors
func (m *MonitoringManager) RecordMetric(metric *Metric) error {
	if !m.config.Enabled {
		return nil
	}
	
	var errs []error
	
	if m.prometheus != nil {
		if err := m.prometheus.RecordMetric(metric); err != nil {
			errs = append(errs, fmt.Errorf("prometheus: %v", err))
		}
	}
	
	if m.datadog != nil {
		if err := m.datadog.RecordMetric(metric); err != nil {
			errs = append(errs, fmt.Errorf("datadog: %v", err))
		}
	}
	
	if m.otel != nil {
		if err := m.otel.RecordMetric(metric); err != nil {
			errs = append(errs, fmt.Errorf("opentelemetry: %v", err))
		}
	}
	
	if m.custom != nil {
		if err := m.custom.RecordMetric(metric); err != nil {
			errs = append(errs, fmt.Errorf("custom: %v", err))
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("monitoring errors: %v", errs)
	}
	
	return nil
}

// RecordRequestMetrics records request metrics across all enabled monitors
func (m *MonitoringManager) RecordRequestMetrics(metrics *RequestMetrics) error {
	if !m.config.Enabled {
		return nil
	}
	
	var errs []error
	
	if m.prometheus != nil {
		if err := m.prometheus.RecordRequestMetrics(metrics); err != nil {
			errs = append(errs, fmt.Errorf("prometheus: %v", err))
		}
	}
	
	if m.datadog != nil {
		if err := m.datadog.RecordRequestMetrics(metrics); err != nil {
			errs = append(errs, fmt.Errorf("datadog: %v", err))
		}
	}
	
	if m.otel != nil {
		if err := m.otel.RecordRequestMetrics(metrics); err != nil {
			errs = append(errs, fmt.Errorf("opentelemetry: %v", err))
		}
	}
	
	if m.custom != nil {
		if err := m.custom.RecordRequestMetrics(metrics); err != nil {
			errs = append(errs, fmt.Errorf("custom: %v", err))
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("monitoring errors: %v", errs)
	}
	
	return nil
}

// RecordError records an error across all enabled monitors
func (m *MonitoringManager) RecordError(errorMsg string, labels map[string]string) error {
	if !m.config.Enabled {
		return nil
	}
	
	var errs []error
	
	if m.prometheus != nil {
		if err := m.prometheus.RecordError(errorMsg, labels); err != nil {
			errs = append(errs, fmt.Errorf("prometheus: %v", err))
		}
	}
	
	if m.datadog != nil {
		if err := m.datadog.RecordError(errorMsg, labels); err != nil {
			errs = append(errs, fmt.Errorf("datadog: %v", err))
		}
	}
	
	if m.otel != nil {
		if err := m.otel.RecordError(errorMsg, labels); err != nil {
			errs = append(errs, fmt.Errorf("opentelemetry: %v", err))
		}
	}
	
	if m.custom != nil {
		if err := m.custom.RecordError(errorMsg, labels); err != nil {
			errs = append(errs, fmt.Errorf("custom: %v", err))
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("monitoring errors: %v", errs)
	}
	
	return nil
}

// StartRequest starts request tracing across all enabled monitors
func (m *MonitoringManager) StartRequest(ctx context.Context, operation string) (context.Context, func()) {
	if !m.config.Enabled {
		return ctx, func() {}
	}
	
	var finishFuncs []func()
	
	if m.prometheus != nil {
		newCtx, finish := m.prometheus.StartRequest(ctx, operation)
		ctx = newCtx
		finishFuncs = append(finishFuncs, finish)
	}
	
	if m.datadog != nil {
		newCtx, finish := m.datadog.StartRequest(ctx, operation)
		ctx = newCtx
		finishFuncs = append(finishFuncs, finish)
	}
	
	if m.otel != nil {
		newCtx, finish := m.otel.StartRequest(ctx, operation)
		ctx = newCtx
		finishFuncs = append(finishFuncs, finish)
	}
	
	if m.custom != nil {
		newCtx, finish := m.custom.StartRequest(ctx, operation)
		ctx = newCtx
		finishFuncs = append(finishFuncs, finish)
	}
	
	return ctx, func() {
		for _, finish := range finishFuncs {
			finish()
		}
	}
}

// Middleware creates HTTP middleware for request monitoring
func (m *MonitoringManager) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !m.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			
			start := time.Now()
			
			// Start request tracing
			ctx, finish := m.StartRequest(r.Context(), r.URL.Path)
			defer finish()
			
			// Wrap response writer to capture status code
			wrapped := &responseWriterWrapper{ResponseWriter: w, statusCode: 200}
			
			// Execute request
			next.ServeHTTP(wrapped, r.WithContext(ctx))
			
			// Record request metrics
			duration := time.Since(start)
			m.RecordRequestMetrics(&RequestMetrics{
				Endpoint:   r.URL.Path,
				Method:     r.Method,
				StatusCode: wrapped.statusCode,
				Duration:   duration,
			})
		})
	}
}

// responseWriterWrapper wraps http.ResponseWriter to capture status code
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Flush flushes all monitors
func (m *MonitoringManager) Flush() error {
	if !m.config.Enabled {
		return nil
	}
	
	var errs []error
	
	if m.prometheus != nil {
		if err := m.prometheus.Flush(); err != nil {
			errs = append(errs, fmt.Errorf("prometheus: %v", err))
		}
	}
	
	if m.datadog != nil {
		if err := m.datadog.Flush(); err != nil {
			errs = append(errs, fmt.Errorf("datadog: %v", err))
		}
	}
	
	if m.otel != nil {
		if err := m.otel.Flush(); err != nil {
			errs = append(errs, fmt.Errorf("opentelemetry: %v", err))
		}
	}
	
	if m.custom != nil {
		if err := m.custom.Flush(); err != nil {
			errs = append(errs, fmt.Errorf("custom: %v", err))
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("monitoring flush errors: %v", errs)
	}
	
	return nil
}

// Close closes all monitors
func (m *MonitoringManager) Close() error {
	if !m.config.Enabled {
		return nil
	}
	
	var errs []error
	
	if m.prometheus != nil {
		if err := m.prometheus.Close(); err != nil {
			errs = append(errs, fmt.Errorf("prometheus: %v", err))
		}
	}
	
	if m.datadog != nil {
		if err := m.datadog.Close(); err != nil {
			errs = append(errs, fmt.Errorf("datadog: %v", err))
		}
	}
	
	if m.otel != nil {
		if err := m.otel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("opentelemetry: %v", err))
		}
	}
	
	if m.custom != nil {
		if err := m.custom.Close(); err != nil {
			errs = append(errs, fmt.Errorf("custom: %v", err))
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("monitoring close errors: %v", errs)
	}
	
	return nil
}

// GetPrometheusHandler returns the Prometheus metrics handler if enabled
func (m *MonitoringManager) GetPrometheusHandler() http.Handler {
	if m.prometheus != nil {
		return m.prometheus.GetHandler()
	}
	return nil
}

// DefaultMonitoringConfig returns a default monitoring configuration
func DefaultMonitoringConfig() *MonitoringConfig {
	return &MonitoringConfig{
		Enabled: true,
		Prometheus: &PrometheusConfig{
			Enabled:   true,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "ogem",
			Subsystem: "proxy",
		},
		TracingSampleRate: 0.1, // 10% sampling
		DetailedMetrics:   true,
	}
}

// HealthMetrics represents system health metrics
type HealthMetrics struct {
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	Uptime      time.Duration
	Version     string
}

// RecordHealthMetrics records system health metrics
func (m *MonitoringManager) RecordHealthMetrics(health *HealthMetrics) error {
	if !m.config.Enabled {
		return nil
	}
	
	metrics := []*Metric{
		{
			Name:      "system_cpu_usage",
			Type:      MetricTypeGauge,
			Value:     health.CPUUsage,
			Timestamp: time.Now(),
		},
		{
			Name:      "system_memory_usage",
			Type:      MetricTypeGauge,
			Value:     health.MemoryUsage,
			Timestamp: time.Now(),
		},
		{
			Name:      "system_disk_usage",
			Type:      MetricTypeGauge,
			Value:     health.DiskUsage,
			Timestamp: time.Now(),
		},
		{
			Name:      "system_uptime_seconds",
			Type:      MetricTypeGauge,
			Value:     health.Uptime.Seconds(),
			Timestamp: time.Now(),
		},
	}
	
	for _, metric := range metrics {
		if err := m.RecordMetric(metric); err != nil {
			return err
		}
	}
	
	return nil
}