package monitoring

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

// OpenTelemetryMonitor implements monitoring using OpenTelemetry
type OpenTelemetryMonitor struct {
	config         *OpenTelemetryConfig
	logger         *zap.SugaredLogger
	meterProvider  *sdkmetric.MeterProvider
	tracerProvider *sdktrace.TracerProvider
	meter          metric.Meter
	tracer         trace.Tracer
	
	// Standard metrics
	requestCounter    metric.Int64Counter
	requestDuration   metric.Float64Histogram
	tokenCounter      metric.Int64Counter
	costCounter       metric.Float64Counter
	errorCounter      metric.Int64Counter
	cacheHitCounter   metric.Int64Counter
	activeConnections metric.Int64UpDownCounter
	queueSize         metric.Int64UpDownCounter
	
	// Custom metrics
	customCounters   map[string]metric.Int64Counter
	customGauges     map[string]metric.Float64UpDownCounter
	customHistograms map[string]metric.Float64Histogram
}

// NewOpenTelemetryMonitor creates a new OpenTelemetry monitor
func NewOpenTelemetryMonitor(config *OpenTelemetryConfig, logger *zap.SugaredLogger) (*OpenTelemetryMonitor, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("OpenTelemetry endpoint is required")
	}
	
	otelMonitor := &OpenTelemetryMonitor{
		config:           config,
		logger:           logger,
		customCounters:   make(map[string]metric.Int64Counter),
		customGauges:     make(map[string]metric.Float64UpDownCounter),
		customHistograms: make(map[string]metric.Float64Histogram),
	}
	
	// Initialize resource
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %v", err)
	}
	
	// Initialize metrics
	if err := otelMonitor.initializeMetrics(res); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %v", err)
	}
	
	// Initialize tracing
	if err := otelMonitor.initializeTracing(res); err != nil {
		return nil, fmt.Errorf("failed to initialize tracing: %v", err)
	}
	
	return otelMonitor, nil
}

// initializeMetrics initializes OpenTelemetry metrics
func (o *OpenTelemetryMonitor) initializeMetrics(res *resource.Resource) error {
	// Create OTLP metrics exporter
	exporter, err := otlpmetricgrpc.New(
		context.Background(),
		otlpmetricgrpc.WithEndpoint(o.config.Endpoint),
		otlpmetricgrpc.WithInsecure(), // TODO: Make this configurable
		otlpmetricgrpc.WithHeaders(o.config.Headers),
	)
	if err != nil {
		return fmt.Errorf("failed to create OTLP metrics exporter: %v", err)
	}
	
	// Create meter provider
	o.meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
	)
	
	// Set global meter provider
	otel.SetMeterProvider(o.meterProvider)
	
	// Create meter
	o.meter = o.meterProvider.Meter("ogem-proxy")
	
	// Initialize standard metrics
	return o.createStandardMetrics()
}

// initializeTracing initializes OpenTelemetry tracing
func (o *OpenTelemetryMonitor) initializeTracing(res *resource.Resource) error {
	// Create OTLP trace exporter
	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpoint(o.config.Endpoint),
		otlptracehttp.WithInsecure(), // TODO: Make this configurable
		otlptracehttp.WithHeaders(o.config.Headers),
	)
	if err != nil {
		return fmt.Errorf("failed to create OTLP trace exporter: %v", err)
	}
	
	// Create tracer provider
	o.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.1)), // 10% sampling
	)
	
	// Set global tracer provider
	otel.SetTracerProvider(o.tracerProvider)
	
	// Create tracer
	o.tracer = o.tracerProvider.Tracer("ogem-proxy")
	
	return nil
}

// createStandardMetrics creates standard metrics
func (o *OpenTelemetryMonitor) createStandardMetrics() error {
	var err error
	
	// Request counter
	o.requestCounter, err = o.meter.Int64Counter(
		"ogem_requests_total",
		metric.WithDescription("Total number of requests"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request counter: %v", err)
	}
	
	// Request duration histogram
	o.requestDuration, err = o.meter.Float64Histogram(
		"ogem_request_duration_seconds",
		metric.WithDescription("Request duration in seconds"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request duration histogram: %v", err)
	}
	
	// Token counter
	o.tokenCounter, err = o.meter.Int64Counter(
		"ogem_tokens_total",
		metric.WithDescription("Total number of tokens processed"),
	)
	if err != nil {
		return fmt.Errorf("failed to create token counter: %v", err)
	}
	
	// Cost counter
	o.costCounter, err = o.meter.Float64Counter(
		"ogem_cost_total",
		metric.WithDescription("Total cost of requests"),
	)
	if err != nil {
		return fmt.Errorf("failed to create cost counter: %v", err)
	}
	
	// Error counter
	o.errorCounter, err = o.meter.Int64Counter(
		"ogem_errors_total",
		metric.WithDescription("Total number of errors"),
	)
	if err != nil {
		return fmt.Errorf("failed to create error counter: %v", err)
	}
	
	// Cache hit counter
	o.cacheHitCounter, err = o.meter.Int64Counter(
		"ogem_cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
	)
	if err != nil {
		return fmt.Errorf("failed to create cache hit counter: %v", err)
	}
	
	// Active connections gauge
	o.activeConnections, err = o.meter.Int64UpDownCounter(
		"ogem_active_connections",
		metric.WithDescription("Number of active connections"),
	)
	if err != nil {
		return fmt.Errorf("failed to create active connections gauge: %v", err)
	}
	
	// Queue size gauge
	o.queueSize, err = o.meter.Int64UpDownCounter(
		"ogem_queue_size",
		metric.WithDescription("Current queue size"),
	)
	if err != nil {
		return fmt.Errorf("failed to create queue size gauge: %v", err)
	}
	
	return nil
}

// RecordMetric records a custom metric
func (o *OpenTelemetryMonitor) RecordMetric(metric *Metric) error {
	ctx := context.Background()
	attrs := o.convertLabelsToAttributes(metric.Labels)
	
	switch metric.Type {
	case MetricTypeCounter:
		return o.recordCounterMetric(ctx, metric.Name, int64(metric.Value), attrs)
	case MetricTypeGauge:
		return o.recordGaugeMetric(ctx, metric.Name, metric.Value, attrs)
	case MetricTypeHistogram:
		return o.recordHistogramMetric(ctx, metric.Name, metric.Value, attrs)
	default:
		return fmt.Errorf("unsupported metric type: %s", metric.Type)
	}
}

// recordCounterMetric records a counter metric
func (o *OpenTelemetryMonitor) recordCounterMetric(ctx context.Context, name string, value int64, attrs []attribute.KeyValue) error {
	counter, exists := o.customCounters[name]
	if !exists {
		var err error
		counter, err = o.meter.Int64Counter(
			name,
			metric.WithDescription(fmt.Sprintf("Custom counter metric: %s", name)),
		)
		if err != nil {
			return fmt.Errorf("failed to create counter %s: %v", name, err)
		}
		o.customCounters[name] = counter
	}
	
	counter.Add(ctx, value, metric.WithAttributes(attrs...))
	return nil
}

// recordGaugeMetric records a gauge metric
func (o *OpenTelemetryMonitor) recordGaugeMetric(ctx context.Context, name string, value float64, attrs []attribute.KeyValue) error {
	gauge, exists := o.customGauges[name]
	if !exists {
		var err error
		gauge, err = o.meter.Float64UpDownCounter(
			name,
			metric.WithDescription(fmt.Sprintf("Custom gauge metric: %s", name)),
		)
		if err != nil {
			return fmt.Errorf("failed to create gauge %s: %v", name, err)
		}
		o.customGauges[name] = gauge
	}
	
	gauge.Add(ctx, value, metric.WithAttributes(attrs...))
	return nil
}

// recordHistogramMetric records a histogram metric
func (o *OpenTelemetryMonitor) recordHistogramMetric(ctx context.Context, name string, value float64, attrs []attribute.KeyValue) error {
	histogram, exists := o.customHistograms[name]
	if !exists {
		var err error
		histogram, err = o.meter.Float64Histogram(
			name,
			metric.WithDescription(fmt.Sprintf("Custom histogram metric: %s", name)),
		)
		if err != nil {
			return fmt.Errorf("failed to create histogram %s: %v", name, err)
		}
		o.customHistograms[name] = histogram
	}
	
	histogram.Record(ctx, value, metric.WithAttributes(attrs...))
	return nil
}

// RecordRequestMetrics records request-specific metrics
func (o *OpenTelemetryMonitor) RecordRequestMetrics(metrics *RequestMetrics) error {
	ctx := context.Background()
	
	attrs := []attribute.KeyValue{
		attribute.String("provider", metrics.Provider),
		attribute.String("model", metrics.Model),
		attribute.String("endpoint", metrics.Endpoint),
		attribute.String("method", metrics.Method),
		attribute.Int("status_code", metrics.StatusCode),
		attribute.String("user_id", metrics.UserID),
		attribute.String("team_id", metrics.TeamID),
	}
	
	// Record request count
	o.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	
	// Record request duration
	o.requestDuration.Record(ctx, metrics.Duration.Seconds(), metric.WithAttributes(attrs...))
	
	// Record token metrics
	if metrics.InputTokens > 0 {
		tokenAttrs := append(attrs, attribute.String("token_type", "input"))
		o.tokenCounter.Add(ctx, metrics.InputTokens, metric.WithAttributes(tokenAttrs...))
	}
	
	if metrics.OutputTokens > 0 {
		tokenAttrs := append(attrs, attribute.String("token_type", "output"))
		o.tokenCounter.Add(ctx, metrics.OutputTokens, metric.WithAttributes(tokenAttrs...))
	}
	
	if metrics.TotalTokens > 0 {
		tokenAttrs := append(attrs, attribute.String("token_type", "total"))
		o.tokenCounter.Add(ctx, metrics.TotalTokens, metric.WithAttributes(tokenAttrs...))
	}
	
	// Record cost
	if metrics.Cost > 0 {
		o.costCounter.Add(ctx, metrics.Cost, metric.WithAttributes(attrs...))
	}
	
	// Record cache hit
	cacheAttrs := []attribute.KeyValue{
		attribute.String("cache_type", "response"),
		attribute.Bool("hit", metrics.CacheHit),
	}
	o.cacheHitCounter.Add(ctx, 1, metric.WithAttributes(cacheAttrs...))
	
	return nil
}

// RecordError records error metrics
func (o *OpenTelemetryMonitor) RecordError(errorMsg string, labels map[string]string) error {
	ctx := context.Background()
	
	attrs := []attribute.KeyValue{
		attribute.String("error_type", errorMsg),
	}
	
	// Add labels as attributes
	for key, value := range labels {
		attrs = append(attrs, attribute.String(key, value))
	}
	
	o.errorCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	return nil
}

// StartRequest starts request tracing
func (o *OpenTelemetryMonitor) StartRequest(ctx context.Context, operation string) (context.Context, func()) {
	ctx, span := o.tracer.Start(ctx, operation)
	
	return ctx, func() {
		span.End()
	}
}

// convertLabelsToAttributes converts labels map to OpenTelemetry attributes
func (o *OpenTelemetryMonitor) convertLabelsToAttributes(labels map[string]string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(labels))
	
	for key, value := range labels {
		attrs = append(attrs, attribute.String(key, value))
	}
	
	return attrs
}

// UpdateSystemMetrics updates system-level metrics
func (o *OpenTelemetryMonitor) UpdateSystemMetrics(activeConns int, queueSize int) {
	ctx := context.Background()
	
	o.activeConnections.Add(ctx, int64(activeConns))
	o.queueSize.Add(ctx, int64(queueSize))
}

// CreateSpan creates a new span
func (o *OpenTelemetryMonitor) CreateSpan(ctx context.Context, name string, attrs map[string]string) (context.Context, trace.Span) {
	spanAttrs := make([]attribute.KeyValue, 0, len(attrs))
	for key, value := range attrs {
		spanAttrs = append(spanAttrs, attribute.String(key, value))
	}
	
	ctx, span := o.tracer.Start(ctx, name, trace.WithAttributes(spanAttrs...))
	return ctx, span
}

// AddSpanEvent adds an event to the current span
func (o *OpenTelemetryMonitor) AddSpanEvent(ctx context.Context, name string, attrs map[string]string) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		spanAttrs := make([]attribute.KeyValue, 0, len(attrs))
		for key, value := range attrs {
			spanAttrs = append(spanAttrs, attribute.String(key, value))
		}
		span.AddEvent(name, trace.WithAttributes(spanAttrs...))
	}
}

// SetSpanError sets an error on the current span
func (o *OpenTelemetryMonitor) SetSpanError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// Flush flushes metrics and traces
func (o *OpenTelemetryMonitor) Flush() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Flush metrics
	if err := o.meterProvider.ForceFlush(ctx); err != nil {
		return fmt.Errorf("failed to flush metrics: %v", err)
	}
	
	// Flush traces
	if err := o.tracerProvider.ForceFlush(ctx); err != nil {
		return fmt.Errorf("failed to flush traces: %v", err)
	}
	
	return nil
}

// Close closes the monitor
func (o *OpenTelemetryMonitor) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Shutdown metrics
	if err := o.meterProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown meter provider: %v", err)
	}
	
	// Shutdown traces
	if err := o.tracerProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown tracer provider: %v", err)
	}
	
	return nil
}