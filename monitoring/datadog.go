package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// DatadogMonitor implements monitoring using Datadog
type DatadogMonitor struct {
	config     *DatadogConfig
	logger     *zap.SugaredLogger
	httpClient *http.Client
	baseURL    string
}

// DatadogMetric represents a Datadog metric
type DatadogMetric struct {
	Metric string      `json:"metric"`
	Points [][]float64 `json:"points"`
	Type   string      `json:"type,omitempty"`
	Host   string      `json:"host,omitempty"`
	Tags   []string    `json:"tags,omitempty"`
}

// DatadogMetricsPayload represents the payload for Datadog metrics API
type DatadogMetricsPayload struct {
	Series []DatadogMetric `json:"series"`
}

// DatadogEvent represents a Datadog event
type DatadogEvent struct {
	Title     string   `json:"title"`
	Text      string   `json:"text"`
	Timestamp int64    `json:"date_happened,omitempty"`
	Priority  string   `json:"priority,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	AlertType string   `json:"alert_type,omitempty"`
}

// DatadogLog represents a Datadog log entry
type DatadogLog struct {
	Timestamp int64                  `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Service   string                 `json:"service"`
	Source    string                 `json:"source"`
	Tags      map[string]interface{} `json:"tags,omitempty"`
}

// NewDatadogMonitor creates a new Datadog monitor
func NewDatadogMonitor(config *DatadogConfig, logger *zap.SugaredLogger) (*DatadogMonitor, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Datadog API key is required")
	}
	
	baseURL := fmt.Sprintf("https://api.%s", config.Site)
	if config.Site == "" {
		baseURL = "https://api.datadoghq.com"
	}
	
	return &DatadogMonitor{
		config: config,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}, nil
}

// RecordMetric records a metric to Datadog
func (d *DatadogMonitor) RecordMetric(metric *Metric) error {
	ddMetric := DatadogMetric{
		Metric: metric.Name,
		Points: [][]float64{{float64(metric.Timestamp.Unix()), metric.Value}},
		Type:   d.convertMetricType(metric.Type),
		Tags:   d.convertLabelsToTags(metric.Labels),
	}
	
	payload := DatadogMetricsPayload{
		Series: []DatadogMetric{ddMetric},
	}
	
	return d.sendMetrics(payload)
}

// RecordRequestMetrics records request-specific metrics to Datadog
func (d *DatadogMonitor) RecordRequestMetrics(metrics *RequestMetrics) error {
	now := float64(time.Now().Unix())
	
	baseTags := []string{
		fmt.Sprintf("provider:%s", metrics.Provider),
		fmt.Sprintf("model:%s", metrics.Model),
		fmt.Sprintf("endpoint:%s", metrics.Endpoint),
		fmt.Sprintf("method:%s", metrics.Method),
		fmt.Sprintf("status_code:%d", metrics.StatusCode),
		fmt.Sprintf("service:%s", d.config.Service),
		fmt.Sprintf("env:%s", d.config.Env),
		fmt.Sprintf("version:%s", d.config.Version),
	}
	
	// Add user and team tags if available
	if metrics.UserID != "" {
		baseTags = append(baseTags, fmt.Sprintf("user_id:%s", metrics.UserID))
	}
	if metrics.TeamID != "" {
		baseTags = append(baseTags, fmt.Sprintf("team_id:%s", metrics.TeamID))
	}
	
	// Add custom tags from config
	baseTags = append(baseTags, d.config.Tags...)
	
	var ddMetrics []DatadogMetric
	
	// Request count
	ddMetrics = append(ddMetrics, DatadogMetric{
		Metric: "ogem.requests.total",
		Points: [][]float64{{now, 1}},
		Type:   "count",
		Tags:   baseTags,
	})
	
	// Request duration
	ddMetrics = append(ddMetrics, DatadogMetric{
		Metric: "ogem.request.duration",
		Points: [][]float64{{now, metrics.Duration.Seconds()}},
		Type:   "gauge",
		Tags:   baseTags,
	})
	
	// Token metrics
	if metrics.InputTokens > 0 {
		tokenTags := append(baseTags, "token_type:input")
		ddMetrics = append(ddMetrics, DatadogMetric{
			Metric: "ogem.tokens.total",
			Points: [][]float64{{now, float64(metrics.InputTokens)}},
			Type:   "count",
			Tags:   tokenTags,
		})
	}
	
	if metrics.OutputTokens > 0 {
		tokenTags := append(baseTags, "token_type:output")
		ddMetrics = append(ddMetrics, DatadogMetric{
			Metric: "ogem.tokens.total",
			Points: [][]float64{{now, float64(metrics.OutputTokens)}},
			Type:   "count",
			Tags:   tokenTags,
		})
	}
	
	if metrics.TotalTokens > 0 {
		tokenTags := append(baseTags, "token_type:total")
		ddMetrics = append(ddMetrics, DatadogMetric{
			Metric: "ogem.tokens.total",
			Points: [][]float64{{now, float64(metrics.TotalTokens)}},
			Type:   "count",
			Tags:   tokenTags,
		})
	}
	
	// Cost metrics
	if metrics.Cost > 0 {
		ddMetrics = append(ddMetrics, DatadogMetric{
			Metric: "ogem.cost.total",
			Points: [][]float64{{now, metrics.Cost}},
			Type:   "count",
			Tags:   baseTags,
		})
	}
	
	// Cache hit metrics
	cacheHitValue := float64(0)
	if metrics.CacheHit {
		cacheHitValue = 1
	}
	cacheTags := append(baseTags, fmt.Sprintf("cache_hit:%t", metrics.CacheHit))
	ddMetrics = append(ddMetrics, DatadogMetric{
		Metric: "ogem.cache.hits",
		Points: [][]float64{{now, cacheHitValue}},
		Type:   "count",
		Tags:   cacheTags,
	})
	
	payload := DatadogMetricsPayload{
		Series: ddMetrics,
	}
	
	return d.sendMetrics(payload)
}

// RecordError records error metrics to Datadog
func (d *DatadogMonitor) RecordError(errorMsg string, labels map[string]string) error {
	tags := []string{
		fmt.Sprintf("error_type:%s", errorMsg),
		fmt.Sprintf("service:%s", d.config.Service),
		fmt.Sprintf("env:%s", d.config.Env),
	}
	
	// Add labels as tags
	for key, value := range labels {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value))
	}
	
	// Add custom tags from config
	tags = append(tags, d.config.Tags...)
	
	ddMetric := DatadogMetric{
		Metric: "ogem.errors.total",
		Points: [][]float64{{float64(time.Now().Unix()), 1}},
		Type:   "count",
		Tags:   tags,
	}
	
	payload := DatadogMetricsPayload{
		Series: []DatadogMetric{ddMetric},
	}
	
	// Also send as an event
	go d.sendErrorEvent(errorMsg, labels)
	
	return d.sendMetrics(payload)
}

// StartRequest starts request tracing for Datadog APM (simplified implementation)
func (d *DatadogMonitor) StartRequest(ctx context.Context, operation string) (context.Context, func()) {
	start := time.Now()
	
	return ctx, func() {
		duration := time.Since(start)
		
		// Record operation duration
		ddMetric := DatadogMetric{
			Metric: "ogem.operation.duration",
			Points: [][]float64{{float64(time.Now().Unix()), duration.Seconds()}},
			Type:   "gauge",
			Tags: []string{
				fmt.Sprintf("operation:%s", operation),
				fmt.Sprintf("service:%s", d.config.Service),
				fmt.Sprintf("env:%s", d.config.Env),
			},
		}
		
		payload := DatadogMetricsPayload{
			Series: []DatadogMetric{ddMetric},
		}
		
		// Send asynchronously to avoid blocking
		go func() {
			if err := d.sendMetrics(payload); err != nil {
				d.logger.Errorw("Failed to send operation duration metric", "error", err)
			}
		}()
	}
}

// sendMetrics sends metrics to Datadog
func (d *DatadogMonitor) sendMetrics(payload DatadogMetricsPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %v", err)
	}
	
	url := fmt.Sprintf("%s/api/v1/series", d.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", d.config.APIKey)
	if d.config.AppKey != "" {
		req.Header.Set("DD-APPLICATION-KEY", d.config.AppKey)
	}
	
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send metrics: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Datadog API returned status %d", resp.StatusCode)
	}
	
	return nil
}

// sendErrorEvent sends an error event to Datadog
func (d *DatadogMonitor) sendErrorEvent(errorMsg string, labels map[string]string) {
	tags := []string{
		fmt.Sprintf("service:%s", d.config.Service),
		fmt.Sprintf("env:%s", d.config.Env),
	}
	
	for key, value := range labels {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value))
	}
	tags = append(tags, d.config.Tags...)
	
	event := DatadogEvent{
		Title:     "Ogem Proxy Error",
		Text:      fmt.Sprintf("Error occurred: %s", errorMsg),
		Timestamp: time.Now().Unix(),
		Priority:  "normal",
		Tags:      tags,
		AlertType: "error",
	}
	
	jsonData, err := json.Marshal(event)
	if err != nil {
		d.logger.Errorw("Failed to marshal error event", "error", err)
		return
	}
	
	url := fmt.Sprintf("%s/api/v1/events", d.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		d.logger.Errorw("Failed to create error event request", "error", err)
		return
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", d.config.APIKey)
	
	resp, err := d.httpClient.Do(req)
	if err != nil {
		d.logger.Errorw("Failed to send error event", "error", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		d.logger.Errorw("Datadog events API returned error", "status", resp.StatusCode)
	}
}

// convertMetricType converts our metric type to Datadog type
func (d *DatadogMonitor) convertMetricType(metricType MetricType) string {
	switch metricType {
	case MetricTypeCounter:
		return "count"
	case MetricTypeGauge:
		return "gauge"
	case MetricTypeHistogram:
		return "histogram"
	case MetricTypeSummary:
		return "distribution"
	default:
		return "gauge"
	}
}

// convertLabelsToTags converts labels map to Datadog tags format
func (d *DatadogMonitor) convertLabelsToTags(labels map[string]string) []string {
	tags := make([]string, 0, len(labels)+len(d.config.Tags)+3)
	
	// Add labels as tags
	for key, value := range labels {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value))
	}
	
	// Add service tags
	tags = append(tags, fmt.Sprintf("service:%s", d.config.Service))
	tags = append(tags, fmt.Sprintf("env:%s", d.config.Env))
	tags = append(tags, fmt.Sprintf("version:%s", d.config.Version))
	
	// Add custom tags from config
	tags = append(tags, d.config.Tags...)
	
	return tags
}

// Flush flushes any pending metrics (no-op for HTTP-based Datadog)
func (d *DatadogMonitor) Flush() error {
	// For HTTP-based implementation, metrics are sent immediately
	// In a production setup, you might want to batch metrics
	return nil
}

// Close closes the monitor
func (d *DatadogMonitor) Close() error {
	// Close HTTP client if needed
	return nil
}

// SendCustomEvent sends a custom event to Datadog
func (d *DatadogMonitor) SendCustomEvent(title, text string, tags map[string]string) error {
	ddTags := d.convertLabelsToTags(tags)
	
	event := DatadogEvent{
		Title:     title,
		Text:      text,
		Timestamp: time.Now().Unix(),
		Priority:  "normal",
		Tags:      ddTags,
		AlertType: "info",
	}
	
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %v", err)
	}
	
	url := fmt.Sprintf("%s/api/v1/events", d.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", d.config.APIKey)
	
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send event: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Datadog API returned status %d", resp.StatusCode)
	}
	
	return nil
}