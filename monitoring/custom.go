package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// CustomMonitor implements monitoring using custom endpoints
type CustomMonitor struct {
	config     *CustomEndpointConfig
	logger     *zap.SugaredLogger
	httpClient *http.Client
}

// CustomMetricPayload represents the payload structure for custom metrics
type CustomMetricPayload struct {
	Timestamp int64                  `json:"timestamp"`
	Metrics   []CustomMetricEntry    `json:"metrics"`
	Tags      map[string]string      `json:"tags,omitempty"`
	Source    string                 `json:"source"`
}

// CustomMetricEntry represents a single metric entry
type CustomMetricEntry struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Value  float64           `json:"value"`
	Unit   string            `json:"unit,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

// InfluxDBLineProtocol represents metrics in InfluxDB line protocol format
type InfluxDBLineProtocol struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]interface{}
	Timestamp   time.Time
}

// StatsDMetric represents a StatsD metric
type StatsDMetric struct {
	Name       string
	Value      float64
	Type       string // c, g, h, ms
	SampleRate float64
	Tags       []string
}

// NewCustomMonitor creates a new custom monitor
func NewCustomMonitor(config *CustomEndpointConfig, logger *zap.SugaredLogger) (*CustomMonitor, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("custom endpoint URL is required")
	}
	
	return &CustomMonitor{
		config: config,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// RecordMetric records a metric using the custom endpoint
func (c *CustomMonitor) RecordMetric(metric *Metric) error {
	switch strings.ToLower(c.config.Format) {
	case "json":
		return c.sendJSONMetric(metric)
	case "influxdb":
		return c.sendInfluxDBMetric(metric)
	case "statsd":
		return c.sendStatsDMetric(metric)
	default:
		return c.sendJSONMetric(metric) // Default to JSON
	}
}

// RecordRequestMetrics records request-specific metrics
func (c *CustomMonitor) RecordRequestMetrics(metrics *RequestMetrics) error {
	now := time.Now()
	
	baseLabels := map[string]string{
		"provider":    metrics.Provider,
		"model":       metrics.Model,
		"endpoint":    metrics.Endpoint,
		"method":      metrics.Method,
		"status_code": strconv.Itoa(metrics.StatusCode),
		"user_id":     metrics.UserID,
		"team_id":     metrics.TeamID,
	}
	
	var customMetrics []*Metric
	
	// Request count
	customMetrics = append(customMetrics, &Metric{
		Name:      "requests_total",
		Type:      MetricTypeCounter,
		Value:     1,
		Labels:    baseLabels,
		Timestamp: now,
	})
	
	// Request duration
	customMetrics = append(customMetrics, &Metric{
		Name:      "request_duration_seconds",
		Type:      MetricTypeHistogram,
		Value:     metrics.Duration.Seconds(),
		Labels:    baseLabels,
		Timestamp: now,
	})
	
	// Token metrics
	if metrics.InputTokens > 0 {
		tokenLabels := make(map[string]string)
		for k, v := range baseLabels {
			tokenLabels[k] = v
		}
		tokenLabels["token_type"] = "input"
		
		customMetrics = append(customMetrics, &Metric{
			Name:      "tokens_total",
			Type:      MetricTypeCounter,
			Value:     float64(metrics.InputTokens),
			Labels:    tokenLabels,
			Timestamp: now,
		})
	}
	
	if metrics.OutputTokens > 0 {
		tokenLabels := make(map[string]string)
		for k, v := range baseLabels {
			tokenLabels[k] = v
		}
		tokenLabels["token_type"] = "output"
		
		customMetrics = append(customMetrics, &Metric{
			Name:      "tokens_total",
			Type:      MetricTypeCounter,
			Value:     float64(metrics.OutputTokens),
			Labels:    tokenLabels,
			Timestamp: now,
		})
	}
	
	// Cost metrics
	if metrics.Cost > 0 {
		customMetrics = append(customMetrics, &Metric{
			Name:      "cost_total",
			Type:      MetricTypeCounter,
			Value:     metrics.Cost,
			Labels:    baseLabels,
			Timestamp: now,
		})
	}
	
	// Cache hit metrics
	cacheLabels := map[string]string{
		"cache_type": "response",
		"hit":        strconv.FormatBool(metrics.CacheHit),
	}
	customMetrics = append(customMetrics, &Metric{
		Name:      "cache_hits_total",
		Type:      MetricTypeCounter,
		Value:     1,
		Labels:    cacheLabels,
		Timestamp: now,
	})
	
	// Send all metrics
	for _, metric := range customMetrics {
		if err := c.RecordMetric(metric); err != nil {
			return fmt.Errorf("failed to record metric %s: %v", metric.Name, err)
		}
	}
	
	return nil
}

// RecordError records error metrics
func (c *CustomMonitor) RecordError(errorMsg string, labels map[string]string) error {
	errorLabels := make(map[string]string)
	for k, v := range labels {
		errorLabels[k] = v
	}
	errorLabels["error_type"] = errorMsg
	
	metric := &Metric{
		Name:      "errors_total",
		Type:      MetricTypeCounter,
		Value:     1,
		Labels:    errorLabels,
		Timestamp: time.Now(),
	}
	
	return c.RecordMetric(metric)
}

// StartRequest starts request tracing (no-op for custom endpoint)
func (c *CustomMonitor) StartRequest(ctx context.Context, operation string) (context.Context, func()) {
	start := time.Now()
	
	return ctx, func() {
		duration := time.Since(start)
		
		metric := &Metric{
			Name:      "operation_duration_seconds",
			Type:      MetricTypeHistogram,
			Value:     duration.Seconds(),
			Labels:    map[string]string{"operation": operation},
			Timestamp: time.Now(),
		}
		
		// Send asynchronously
		go func() {
			if err := c.RecordMetric(metric); err != nil {
				c.logger.Errorw("Failed to record operation duration", "error", err)
			}
		}()
	}
}

// sendJSONMetric sends a metric in JSON format
func (c *CustomMonitor) sendJSONMetric(metric *Metric) error {
	entry := CustomMetricEntry{
		Name:   metric.Name,
		Type:   string(metric.Type),
		Value:  metric.Value,
		Labels: metric.Labels,
	}
	
	payload := CustomMetricPayload{
		Timestamp: metric.Timestamp.Unix(),
		Metrics:   []CustomMetricEntry{entry},
		Source:    "ogem-proxy",
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON payload: %v", err)
	}
	
	return c.sendHTTPRequest(jsonData, "application/json")
}

// sendInfluxDBMetric sends a metric in InfluxDB line protocol format
func (c *CustomMonitor) sendInfluxDBMetric(metric *Metric) error {
	line := InfluxDBLineProtocol{
		Measurement: metric.Name,
		Tags:        metric.Labels,
		Fields: map[string]interface{}{
			"value": metric.Value,
		},
		Timestamp: metric.Timestamp,
	}
	
	lineProtocol := c.formatInfluxDBLine(line)
	
	return c.sendHTTPRequest([]byte(lineProtocol), "text/plain")
}

// sendStatsDMetric sends a metric in StatsD format
func (c *CustomMonitor) sendStatsDMetric(metric *Metric) error {
	statsdMetric := StatsDMetric{
		Name:       metric.Name,
		Value:      metric.Value,
		Type:       c.convertToStatsDType(metric.Type),
		SampleRate: 1.0,
		Tags:       c.convertLabelsToStatsDTags(metric.Labels),
	}
	
	statsdLine := c.formatStatsDLine(statsdMetric)
	
	return c.sendHTTPRequest([]byte(statsdLine), "text/plain")
}

// sendHTTPRequest sends an HTTP request to the custom endpoint
func (c *CustomMonitor) sendHTTPRequest(data []byte, contentType string) error {
	req, err := http.NewRequest("POST", c.config.URL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	
	req.Header.Set("Content-Type", contentType)
	
	// Add custom headers
	for key, value := range c.config.Headers {
		req.Header.Set(key, value)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("custom endpoint returned status %d", resp.StatusCode)
	}
	
	return nil
}

// formatInfluxDBLine formats a metric in InfluxDB line protocol
func (c *CustomMonitor) formatInfluxDBLine(line InfluxDBLineProtocol) string {
	var builder strings.Builder
	
	// Measurement name
	builder.WriteString(line.Measurement)
	
	// Tags
	if len(line.Tags) > 0 {
		for key, value := range line.Tags {
			builder.WriteString(",")
			builder.WriteString(key)
			builder.WriteString("=")
			builder.WriteString(value)
		}
	}
	
	builder.WriteString(" ")
	
	// Fields
	fieldCount := 0
	for key, value := range line.Fields {
		if fieldCount > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(key)
		builder.WriteString("=")
		
		switch v := value.(type) {
		case string:
			builder.WriteString("\"")
			builder.WriteString(v)
			builder.WriteString("\"")
		case float64:
			builder.WriteString(fmt.Sprintf("%g", v))
		case int64:
			builder.WriteString(fmt.Sprintf("%di", v))
		case bool:
			builder.WriteString(fmt.Sprintf("%t", v))
		default:
			builder.WriteString(fmt.Sprintf("%v", v))
		}
		fieldCount++
	}
	
	// Timestamp (nanoseconds)
	builder.WriteString(" ")
	builder.WriteString(fmt.Sprintf("%d", line.Timestamp.UnixNano()))
	
	return builder.String()
}

// formatStatsDLine formats a metric in StatsD format
func (c *CustomMonitor) formatStatsDLine(metric StatsDMetric) string {
	var builder strings.Builder
	
	// Metric name
	builder.WriteString(metric.Name)
	builder.WriteString(":")
	
	// Value
	builder.WriteString(fmt.Sprintf("%g", metric.Value))
	
	// Type
	builder.WriteString("|")
	builder.WriteString(metric.Type)
	
	// Sample rate (if not 1.0)
	if metric.SampleRate != 1.0 {
		builder.WriteString("|@")
		builder.WriteString(fmt.Sprintf("%g", metric.SampleRate))
	}
	
	// Tags
	if len(metric.Tags) > 0 {
		builder.WriteString("|#")
		for i, tag := range metric.Tags {
			if i > 0 {
				builder.WriteString(",")
			}
			builder.WriteString(tag)
		}
	}
	
	return builder.String()
}

// convertToStatsDType converts metric type to StatsD type
func (c *CustomMonitor) convertToStatsDType(metricType MetricType) string {
	switch metricType {
	case MetricTypeCounter:
		return "c"
	case MetricTypeGauge:
		return "g"
	case MetricTypeHistogram:
		return "h"
	case MetricTypeSummary:
		return "ms"
	default:
		return "g"
	}
}

// convertLabelsToStatsDTags converts labels to StatsD tags
func (c *CustomMonitor) convertLabelsToStatsDTags(labels map[string]string) []string {
	tags := make([]string, 0, len(labels))
	
	for key, value := range labels {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value))
	}
	
	return tags
}

// Flush flushes any pending metrics (no-op for HTTP-based custom endpoint)
func (c *CustomMonitor) Flush() error {
	return nil
}

// Close closes the monitor
func (c *CustomMonitor) Close() error {
	return nil
}

// SendBatchMetrics sends multiple metrics in a single request
func (c *CustomMonitor) SendBatchMetrics(metrics []*Metric) error {
	if len(metrics) == 0 {
		return nil
	}
	
	switch strings.ToLower(c.config.Format) {
	case "json":
		return c.sendJSONBatchMetrics(metrics)
	case "influxdb":
		return c.sendInfluxDBBatchMetrics(metrics)
	case "statsd":
		return c.sendStatsDatchMetrics(metrics)
	default:
		return c.sendJSONBatchMetrics(metrics)
	}
}

// sendJSONBatchMetrics sends multiple metrics in JSON format
func (c *CustomMonitor) sendJSONBatchMetrics(metrics []*Metric) error {
	entries := make([]CustomMetricEntry, len(metrics))
	
	for i, metric := range metrics {
		entries[i] = CustomMetricEntry{
			Name:   metric.Name,
			Type:   string(metric.Type),
			Value:  metric.Value,
			Labels: metric.Labels,
		}
	}
	
	payload := CustomMetricPayload{
		Timestamp: time.Now().Unix(),
		Metrics:   entries,
		Source:    "ogem-proxy",
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON batch payload: %v", err)
	}
	
	return c.sendHTTPRequest(jsonData, "application/json")
}

// sendInfluxDBBatchMetrics sends multiple metrics in InfluxDB line protocol
func (c *CustomMonitor) sendInfluxDBBatchMetrics(metrics []*Metric) error {
	var builder strings.Builder
	
	for i, metric := range metrics {
		if i > 0 {
			builder.WriteString("\n")
		}
		
		line := InfluxDBLineProtocol{
			Measurement: metric.Name,
			Tags:        metric.Labels,
			Fields: map[string]interface{}{
				"value": metric.Value,
			},
			Timestamp: metric.Timestamp,
		}
		
		builder.WriteString(c.formatInfluxDBLine(line))
	}
	
	return c.sendHTTPRequest([]byte(builder.String()), "text/plain")
}

// sendStatsDatchMetrics sends multiple metrics in StatsD format
func (c *CustomMonitor) sendStatsDatchMetrics(metrics []*Metric) error {
	var builder strings.Builder
	
	for i, metric := range metrics {
		if i > 0 {
			builder.WriteString("\n")
		}
		
		statsdMetric := StatsDMetric{
			Name:       metric.Name,
			Value:      metric.Value,
			Type:       c.convertToStatsDType(metric.Type),
			SampleRate: 1.0,
			Tags:       c.convertLabelsToStatsDTags(metric.Labels),
		}
		
		builder.WriteString(c.formatStatsDLine(statsdMetric))
	}
	
	return c.sendHTTPRequest([]byte(builder.String()), "text/plain")
}