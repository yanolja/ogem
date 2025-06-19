package monitoring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewPrometheusMonitor(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name    string
		config  *PrometheusConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &PrometheusConfig{
				Enabled:   true,
				Port:      9090,
				Path:      "/metrics",
				Namespace: "ogem",
				Subsystem: "proxy",
			},
			wantErr: false,
		},
		{
			name: "minimal config",
			config: &PrometheusConfig{
				Enabled:   true,
				Namespace: "test",
				Subsystem: "monitoring",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor, err := NewPrometheusMonitor(tt.config, logger)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, monitor)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, monitor)
				assert.Equal(t, tt.config, monitor.config)
				assert.NotNil(t, monitor.registry)
				assert.NotNil(t, monitor.requestsTotal)
				assert.NotNil(t, monitor.requestDuration)
				assert.NotNil(t, monitor.tokensTotal)
				assert.NotNil(t, monitor.costTotal)
				assert.NotNil(t, monitor.errorsTotal)
				assert.NotNil(t, monitor.cacheHitsTotal)
				assert.NotNil(t, monitor.providerLatency)
				assert.NotNil(t, monitor.customMetrics)
			}
		})
	}
}

func TestPrometheusMonitor_RecordMetric(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	tests := []struct {
		name    string
		metric  *Metric
		wantErr bool
	}{
		{
			name: "counter metric",
			metric: &Metric{
				Name:      "test_counter",
				Type:      MetricTypeCounter,
				Value:     1.0,
				Labels:    map[string]string{"service": "test"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "gauge metric",
			metric: &Metric{
				Name:      "test_gauge",
				Type:      MetricTypeGauge,
				Value:     50.5,
				Labels:    map[string]string{"instance": "node1"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "histogram metric",
			metric: &Metric{
				Name:      "test_histogram",
				Type:      MetricTypeHistogram,
				Value:     0.25,
				Labels:    map[string]string{"method": "GET"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "summary metric",
			metric: &Metric{
				Name:      "test_summary",
				Type:      MetricTypeSummary,
				Value:     1.5,
				Labels:    map[string]string{"handler": "/api/v1"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "unsupported metric type",
			metric: &Metric{
				Name:      "test_invalid",
				Type:      MetricType("invalid"),
				Value:     1.0,
				Labels:    map[string]string{},
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monitor.RecordMetric(tt.metric)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify metric was registered in custom metrics
				if tt.metric.Type != MetricType("invalid") {
					assert.Contains(t, monitor.customMetrics, tt.metric.Name)
				}
			}
		})
	}
}

func TestPrometheusMonitor_RecordRequestMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	tests := []struct {
		name    string
		metrics *RequestMetrics
	}{
		{
			name: "complete request metrics",
			metrics: &RequestMetrics{
				Provider:     "openai",
				Model:        "gpt-4o",
				Endpoint:     "/v1/chat/completions",
				Method:       "POST",
				StatusCode:   200,
				Duration:     100 * time.Millisecond,
				InputTokens:  50,
				OutputTokens: 30,
				TotalTokens:  80,
				Cost:         0.001,
				UserID:       "user-123",
				TeamID:       "team-456",
				CacheHit:     false,
			},
		},
		{
			name: "minimal request metrics",
			metrics: &RequestMetrics{
				Provider:   "anthropic",
				Model:      "claude-3",
				Endpoint:   "/v1/messages",
				Method:     "POST",
				StatusCode: 200,
				Duration:   50 * time.Millisecond,
			},
		},
		{
			name: "error request metrics",
			metrics: &RequestMetrics{
				Provider:   "google",
				Model:      "gemini-2.5-pro",
				Endpoint:   "/v1/generate",
				Method:     "POST",
				StatusCode: 429,
				Duration:   10 * time.Millisecond,
				Error:      "rate_limit_exceeded",
			},
		},
		{
			name: "cached request metrics",
			metrics: &RequestMetrics{
				Provider:     "openai",
				Model:        "gpt-4",
				Endpoint:     "/v1/chat/completions",
				Method:       "POST",
				StatusCode:   200,
				Duration:     5 * time.Millisecond,
				InputTokens:  100,
				OutputTokens: 150,
				TotalTokens:  250,
				Cost:         0.005,
				UserID:       "user-789",
				TeamID:       "team-012",
				CacheHit:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monitor.RecordRequestMetrics(tt.metrics)
			assert.NoError(t, err)
		})
	}
}

func TestPrometheusMonitor_RecordError(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	tests := []struct {
		name     string
		errorMsg string
		labels   map[string]string
	}{
		{
			name:     "authentication error",
			errorMsg: "invalid_api_key",
			labels: map[string]string{
				"provider": "openai",
				"model":    "gpt-3.5-turbo",
				"endpoint": "/v1/chat/completions",
			},
		},
		{
			name:     "rate limit error",
			errorMsg: "rate_limit_exceeded",
			labels: map[string]string{
				"provider": "anthropic",
				"model":    "claude-3",
				"endpoint": "/v1/messages",
			},
		},
		{
			name:     "timeout error",
			errorMsg: "request_timeout",
			labels: map[string]string{
				"provider": "google",
				"model":    "gemini-2.5-pro",
				"endpoint": "/v1/generate",
			},
		},
		{
			name:     "error with nil labels",
			errorMsg: "internal_error",
			labels:   nil,
		},
		{
			name:     "error with empty labels",
			errorMsg: "validation_error",
			labels:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monitor.RecordError(tt.errorMsg, tt.labels)
			assert.NoError(t, err)
		})
	}
}

func TestPrometheusMonitor_StartRequest(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	ctx := context.Background()
	operation := "test_operation"

	newCtx, finish := monitor.StartRequest(ctx, operation)

	assert.NotNil(t, newCtx)
	assert.NotNil(t, finish)

	// Finish should not panic and should complete quickly
	start := time.Now()
	finish()
	duration := time.Since(start)
	assert.Less(t, duration, 100*time.Millisecond)
}

func TestPrometheusMonitor_GetHandler(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	handler := monitor.GetHandler()
	assert.NotNil(t, handler)

	// Test that the handler serves metrics
	req := httptest.NewRequest("GET", "/metrics", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/plain")

	// Should contain prometheus metrics format
	body := recorder.Body.String()
	assert.Contains(t, body, "# HELP")
	assert.Contains(t, body, "# TYPE")
}

func TestPrometheusMonitor_UpdateSystemMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	tests := []struct {
		name        string
		activeConns int
		queueSize   int
	}{
		{
			name:        "normal load",
			activeConns: 50,
			queueSize:   10,
		},
		{
			name:        "high load",
			activeConns: 200,
			queueSize:   50,
		},
		{
			name:        "no load",
			activeConns: 0,
			queueSize:   0,
		},
		{
			name:        "negative values",
			activeConns: -1,
			queueSize:   -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			monitor.UpdateSystemMetrics(tt.activeConns, tt.queueSize)
		})
	}
}

func TestPrometheusMonitor_FlushAndClose(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	// Test Flush
	err = monitor.Flush()
	assert.NoError(t, err)

	// Test Close
	err = monitor.Close()
	assert.NoError(t, err)

	// Multiple calls should not error
	err = monitor.Flush()
	assert.NoError(t, err)

	err = monitor.Close()
	assert.NoError(t, err)
}

func TestPrometheusMonitor_MetricReuse(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	// Record the same metric multiple times to test reuse
	metric := &Metric{
		Name:      "reuse_test",
		Type:      MetricTypeCounter,
		Value:     1.0,
		Labels:    map[string]string{"test": "reuse"},
		Timestamp: time.Now(),
	}

	// First recording should create the metric
	err = monitor.RecordMetric(metric)
	assert.NoError(t, err)
	assert.Contains(t, monitor.customMetrics, metric.Name)

	// Second recording should reuse the existing metric
	metric.Value = 2.0
	err = monitor.RecordMetric(metric)
	assert.NoError(t, err)

	// Third recording with different labels should still work
	metric.Labels = map[string]string{"test": "different"}
	metric.Value = 3.0
	err = monitor.RecordMetric(metric)
	assert.NoError(t, err)
}

func TestPrometheusMonitor_MetricRegistration(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	// Test that standard metrics are registered
	standardMetrics := []string{
		"requests_total",
		"request_duration_seconds",
		"tokens_total",
		"cost_total",
		"errors_total",
		"cache_hits_total",
		"provider_latency_seconds",
		"active_connections",
		"queue_size",
	}

	// Get metrics from registry
	metricFamilies, err := monitor.registry.Gather()
	require.NoError(t, err)

	registeredMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		registeredMetrics[*mf.Name] = true
	}

	// Verify all standard metrics are registered
	for _, metricName := range standardMetrics {
		fullName := config.Namespace + "_" + config.Subsystem + "_" + metricName
		assert.True(t, registeredMetrics[fullName], "Metric %s should be registered", fullName)
	}
}

func TestGetLabelNames(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected []string
	}{
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: []string{},
		},
		{
			name: "single label",
			labels: map[string]string{
				"service": "test",
			},
			expected: []string{"service"},
		},
		{
			name: "multiple labels",
			labels: map[string]string{
				"service":     "test",
				"environment": "dev",
				"version":     "1.0.0",
			},
			expected: []string{"service", "environment", "version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLabelNames(tt.labels)
			
			assert.Equal(t, len(tt.expected), len(result))
			
			// Convert to map for easier comparison (order doesn't matter)
			expectedMap := make(map[string]bool)
			for _, name := range tt.expected {
				expectedMap[name] = true
			}
			
			for _, name := range result {
				assert.True(t, expectedMap[name], "Unexpected label name: %s", name)
			}
		})
	}
}

func TestGetLabel(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		key      string
		expected string
	}{
		{
			name:     "nil labels",
			labels:   nil,
			key:      "service",
			expected: "",
		},
		{
			name:     "empty labels",
			labels:   map[string]string{},
			key:      "service",
			expected: "",
		},
		{
			name: "existing key",
			labels: map[string]string{
				"service": "test",
				"env":     "prod",
			},
			key:      "service",
			expected: "test",
		},
		{
			name: "non-existing key",
			labels: map[string]string{
				"service": "test",
			},
			key:      "environment",
			expected: "",
		},
		{
			name: "empty value",
			labels: map[string]string{
				"service": "",
			},
			key:      "service",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLabel(tt.labels, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrometheusMonitor_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &PrometheusConfig{
		Enabled:   true,
		Namespace: "test",
		Subsystem: "monitoring",
	}

	monitor, err := NewPrometheusMonitor(config, logger)
	require.NoError(t, err)

	// Test concurrent metric recording
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < 100; j++ {
				metric := &Metric{
					Name:      "concurrent_test",
					Type:      MetricTypeCounter,
					Value:     1.0,
					Labels:    map[string]string{"worker": string(rune(id + '0'))},
					Timestamp: time.Now(),
				}

				err := monitor.RecordMetric(metric)
				assert.NoError(t, err)

				// Also test request metrics
				requestMetrics := &RequestMetrics{
					Provider:   "test",
					Model:      "test-model",
					Endpoint:   "/test",
					Method:     "POST",
					StatusCode: 200,
					Duration:   time.Millisecond,
				}

				err = monitor.RecordRequestMetrics(requestMetrics)
				assert.NoError(t, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify the metric was created and is accessible
	assert.Contains(t, monitor.customMetrics, "concurrent_test")
}