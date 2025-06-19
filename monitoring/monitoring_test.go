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

func TestNewMonitoringManager(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name    string
		config  *MonitoringConfig
		wantErr bool
	}{
		{
			name: "disabled monitoring",
			config: &MonitoringConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "prometheus enabled",
			config: &MonitoringConfig{
				Enabled: true,
				Prometheus: &PrometheusConfig{
					Enabled:   true,
					Port:      9090,
					Path:      "/metrics",
					Namespace: "ogem",
					Subsystem: "proxy",
				},
			},
			wantErr: false,
		},
		{
			name: "all providers enabled",
			config: &MonitoringConfig{
				Enabled: true,
				Prometheus: &PrometheusConfig{
					Enabled:   true,
					Port:      9090,
					Path:      "/metrics",
					Namespace: "ogem",
					Subsystem: "proxy",
				},
				Datadog: &DatadogConfig{
					Enabled: true,
					APIKey:  "test-key",
					Site:    "datadoghq.com",
					Service: "ogem",
					Env:     "test",
					Version: "1.0.0",
				},
				OpenTelemetry: &OpenTelemetryConfig{
					Enabled:        true,
					Endpoint:       "http://localhost:4317",
					ServiceName:    "ogem",
					ServiceVersion: "1.0.0",
					Environment:    "test",
				},
				CustomEndpoint: &CustomEndpointConfig{
					Enabled: true,
					URL:     "http://localhost:8080/metrics",
					Format:  "json",
				},
				TracingSampleRate: 0.1,
				DetailedMetrics:   true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewMonitoringManager(tt.config, logger)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				assert.Equal(t, tt.config.Enabled, manager.config.Enabled)

				if tt.config.Enabled {
					if tt.config.Prometheus != nil && tt.config.Prometheus.Enabled {
						assert.NotNil(t, manager.prometheus)
					} else {
						assert.Nil(t, manager.prometheus)
					}
				}
			}
		})
	}
}

func TestDefaultMonitoringConfig(t *testing.T) {
	config := DefaultMonitoringConfig()

	assert.NotNil(t, config)
	assert.True(t, config.Enabled)
	assert.NotNil(t, config.Prometheus)
	assert.True(t, config.Prometheus.Enabled)
	assert.Equal(t, 9090, config.Prometheus.Port)
	assert.Equal(t, "/metrics", config.Prometheus.Path)
	assert.Equal(t, "ogem", config.Prometheus.Namespace)
	assert.Equal(t, "proxy", config.Prometheus.Subsystem)
	assert.Equal(t, 0.1, config.TracingSampleRate)
	assert.True(t, config.DetailedMetrics)
}

func TestMonitoringManager_RecordMetric(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name    string
		enabled bool
		metric  *Metric
		wantErr bool
	}{
		{
			name:    "disabled monitoring",
			enabled: false,
			metric: &Metric{
				Name:      "test_counter",
				Type:      MetricTypeCounter,
				Value:     1.0,
				Labels:    map[string]string{"test": "value"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name:    "enabled monitoring with counter",
			enabled: true,
			metric: &Metric{
				Name:      "test_counter",
				Type:      MetricTypeCounter,
				Value:     1.0,
				Labels:    map[string]string{"test": "value"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name:    "enabled monitoring with gauge",
			enabled: true,
			metric: &Metric{
				Name:      "test_gauge",
				Type:      MetricTypeGauge,
				Value:     50.0,
				Labels:    map[string]string{"test": "value"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name:    "enabled monitoring with histogram",
			enabled: true,
			metric: &Metric{
				Name:      "test_histogram",
				Type:      MetricTypeHistogram,
				Value:     0.5,
				Labels:    map[string]string{"test": "value"},
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &MonitoringConfig{
				Enabled: tt.enabled,
			}

			if tt.enabled {
				config.Prometheus = &PrometheusConfig{
					Enabled:   true,
					Port:      9090,
					Path:      "/metrics",
					Namespace: "test",
					Subsystem: "monitoring",
				}
			}

			manager, err := NewMonitoringManager(config, logger)
			require.NoError(t, err)

			err = manager.RecordMetric(tt.metric)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMonitoringManager_RecordRequestMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &MonitoringConfig{
		Enabled: true,
		Prometheus: &PrometheusConfig{
			Enabled:   true,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "test",
			Subsystem: "monitoring",
		},
	}

	manager, err := NewMonitoringManager(config, logger)
	require.NoError(t, err)

	metrics := &RequestMetrics{
		Provider:     "openai",
		Model:        "gpt-3.5-turbo",
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
		Error:        "",
	}

	err = manager.RecordRequestMetrics(metrics)
	assert.NoError(t, err)

	// Test with cache hit
	metrics.CacheHit = true
	err = manager.RecordRequestMetrics(metrics)
	assert.NoError(t, err)

	// Test with error
	metrics.Error = "rate_limit_exceeded"
	metrics.StatusCode = 429
	err = manager.RecordRequestMetrics(metrics)
	assert.NoError(t, err)
}

func TestMonitoringManager_RecordError(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &MonitoringConfig{
		Enabled: true,
		Prometheus: &PrometheusConfig{
			Enabled:   true,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "test",
			Subsystem: "monitoring",
		},
	}

	manager, err := NewMonitoringManager(config, logger)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RecordError(tt.errorMsg, tt.labels)
			assert.NoError(t, err)
		})
	}
}

func TestMonitoringManager_StartRequest(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "disabled monitoring",
			enabled: false,
		},
		{
			name:    "enabled monitoring",
			enabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &MonitoringConfig{
				Enabled: tt.enabled,
			}

			if tt.enabled {
				config.Prometheus = &PrometheusConfig{
					Enabled:   true,
					Port:      9090,
					Path:      "/metrics",
					Namespace: "test",
					Subsystem: "monitoring",
				}
			}

			manager, err := NewMonitoringManager(config, logger)
			require.NoError(t, err)

			ctx := context.Background()
			newCtx, finish := manager.StartRequest(ctx, "test_operation")

			assert.NotNil(t, newCtx)
			assert.NotNil(t, finish)

			// Finish should not panic
			finish()
		})
	}
}

func TestMonitoringManager_Middleware(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "disabled monitoring",
			enabled: false,
		},
		{
			name:    "enabled monitoring",
			enabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &MonitoringConfig{
				Enabled: tt.enabled,
			}

			if tt.enabled {
				config.Prometheus = &PrometheusConfig{
					Enabled:   true,
					Port:      9090,
					Path:      "/metrics",
					Namespace: "test",
					Subsystem: "monitoring",
				}
			}

			manager, err := NewMonitoringManager(config, logger)
			require.NoError(t, err)

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			})

			// Apply middleware
			middleware := manager.Middleware()
			wrappedHandler := middleware(testHandler)

			// Test request
			req := httptest.NewRequest("GET", "/test", nil)
			recorder := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "OK", recorder.Body.String())
		})
	}
}

func TestMonitoringManager_Flush(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &MonitoringConfig{
		Enabled: true,
		Prometheus: &PrometheusConfig{
			Enabled:   true,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "test",
			Subsystem: "monitoring",
		},
	}

	manager, err := NewMonitoringManager(config, logger)
	require.NoError(t, err)

	err = manager.Flush()
	assert.NoError(t, err)

	// Test with disabled monitoring
	disabledConfig := &MonitoringConfig{
		Enabled: false,
	}

	disabledManager, err := NewMonitoringManager(disabledConfig, logger)
	require.NoError(t, err)

	err = disabledManager.Flush()
	assert.NoError(t, err)
}

func TestMonitoringManager_Close(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &MonitoringConfig{
		Enabled: true,
		Prometheus: &PrometheusConfig{
			Enabled:   true,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "test",
			Subsystem: "monitoring",
		},
	}

	manager, err := NewMonitoringManager(config, logger)
	require.NoError(t, err)

	err = manager.Close()
	assert.NoError(t, err)

	// Test with disabled monitoring
	disabledConfig := &MonitoringConfig{
		Enabled: false,
	}

	disabledManager, err := NewMonitoringManager(disabledConfig, logger)
	require.NoError(t, err)

	err = disabledManager.Close()
	assert.NoError(t, err)
}

func TestMonitoringManager_GetPrometheusHandler(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name            string
		prometheusEnabled bool
		expectHandler   bool
	}{
		{
			name:            "prometheus enabled",
			prometheusEnabled: true,
			expectHandler:   true,
		},
		{
			name:            "prometheus disabled",
			prometheusEnabled: false,
			expectHandler:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &MonitoringConfig{
				Enabled: true,
			}

			if tt.prometheusEnabled {
				config.Prometheus = &PrometheusConfig{
					Enabled:   true,
					Port:      9090,
					Path:      "/metrics",
					Namespace: "test",
					Subsystem: "monitoring",
				}
			}

			manager, err := NewMonitoringManager(config, logger)
			require.NoError(t, err)

			handler := manager.GetPrometheusHandler()

			if tt.expectHandler {
				assert.NotNil(t, handler)
			} else {
				assert.Nil(t, handler)
			}
		})
	}
}

func TestMonitoringManager_RecordHealthMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &MonitoringConfig{
		Enabled: true,
		Prometheus: &PrometheusConfig{
			Enabled:   true,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "test",
			Subsystem: "monitoring",
		},
	}

	manager, err := NewMonitoringManager(config, logger)
	require.NoError(t, err)

	health := &HealthMetrics{
		CPUUsage:    75.5,
		MemoryUsage: 60.2,
		DiskUsage:   45.8,
		Uptime:      24 * time.Hour,
		Version:     "1.0.0",
	}

	err = manager.RecordHealthMetrics(health)
	assert.NoError(t, err)

	// Test with disabled monitoring
	disabledConfig := &MonitoringConfig{
		Enabled: false,
	}

	disabledManager, err := NewMonitoringManager(disabledConfig, logger)
	require.NoError(t, err)

	err = disabledManager.RecordHealthMetrics(health)
	assert.NoError(t, err)
}

func TestResponseWriterWrapper(t *testing.T) {
	recorder := httptest.NewRecorder()
	wrapper := &responseWriterWrapper{
		ResponseWriter: recorder,
		statusCode:     200, // Default status code
	}

	// Test default status code
	assert.Equal(t, 200, wrapper.statusCode)

	// Test WriteHeader
	wrapper.WriteHeader(404)
	assert.Equal(t, 404, wrapper.statusCode)
	assert.Equal(t, 404, recorder.Code)

	// Test Write
	data := []byte("test response")
	n, err := wrapper.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, "test response", recorder.Body.String())
}

func TestMetricType_Constants(t *testing.T) {
	assert.Equal(t, MetricType("counter"), MetricTypeCounter)
	assert.Equal(t, MetricType("gauge"), MetricTypeGauge)
	assert.Equal(t, MetricType("histogram"), MetricTypeHistogram)
	assert.Equal(t, MetricType("summary"), MetricTypeSummary)
}

func TestMetric_Structure(t *testing.T) {
	now := time.Now()
	metric := &Metric{
		Name:      "test_metric",
		Type:      MetricTypeCounter,
		Value:     42.0,
		Labels:    map[string]string{"env": "test", "service": "ogem"},
		Timestamp: now,
	}

	assert.Equal(t, "test_metric", metric.Name)
	assert.Equal(t, MetricTypeCounter, metric.Type)
	assert.Equal(t, 42.0, metric.Value)
	assert.Equal(t, "test", metric.Labels["env"])
	assert.Equal(t, "ogem", metric.Labels["service"])
	assert.Equal(t, now, metric.Timestamp)
}

func TestRequestMetrics_Structure(t *testing.T) {
	metrics := &RequestMetrics{
		Provider:     "openai",
		Model:        "gpt-4",
		Endpoint:     "/v1/chat/completions",
		Method:       "POST",
		StatusCode:   200,
		Duration:     250 * time.Millisecond,
		InputTokens:  100,
		OutputTokens: 75,
		TotalTokens:  175,
		Cost:         0.0035,
		UserID:       "user-789",
		TeamID:       "team-123",
		CacheHit:     true,
		Error:        "",
	}

	assert.Equal(t, "openai", metrics.Provider)
	assert.Equal(t, "gpt-4", metrics.Model)
	assert.Equal(t, "/v1/chat/completions", metrics.Endpoint)
	assert.Equal(t, "POST", metrics.Method)
	assert.Equal(t, 200, metrics.StatusCode)
	assert.Equal(t, 250*time.Millisecond, metrics.Duration)
	assert.Equal(t, int64(100), metrics.InputTokens)
	assert.Equal(t, int64(75), metrics.OutputTokens)
	assert.Equal(t, int64(175), metrics.TotalTokens)
	assert.Equal(t, 0.0035, metrics.Cost)
	assert.Equal(t, "user-789", metrics.UserID)
	assert.Equal(t, "team-123", metrics.TeamID)
	assert.True(t, metrics.CacheHit)
	assert.Empty(t, metrics.Error)
}

func TestHealthMetrics_Structure(t *testing.T) {
	uptime := 72 * time.Hour
	health := &HealthMetrics{
		CPUUsage:    85.2,
		MemoryUsage: 92.7,
		DiskUsage:   67.3,
		Uptime:      uptime,
		Version:     "2.1.0",
	}

	assert.Equal(t, 85.2, health.CPUUsage)
	assert.Equal(t, 92.7, health.MemoryUsage)
	assert.Equal(t, 67.3, health.DiskUsage)
	assert.Equal(t, uptime, health.Uptime)
	assert.Equal(t, "2.1.0", health.Version)
}

func TestMonitoringConfig_Validation(t *testing.T) {
	config := &MonitoringConfig{
		Enabled: true,
		Prometheus: &PrometheusConfig{
			Enabled:   true,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "ogem",
			Subsystem: "proxy",
		},
		Datadog: &DatadogConfig{
			Enabled: true,
			APIKey:  "test-dd-key",
			AppKey:  "test-app-key",
			Site:    "datadoghq.eu",
			Service: "ogem-proxy",
			Env:     "production",
			Version: "1.2.3",
			Tags:    []string{"team:backend", "component:proxy"},
		},
		OpenTelemetry: &OpenTelemetryConfig{
			Enabled:        true,
			Endpoint:       "https://api.honeycomb.io:443",
			ServiceName:    "ogem-proxy",
			ServiceVersion: "1.2.3",
			Environment:    "production",
			Headers: map[string]string{
				"x-honeycomb-team": "test-key",
			},
			Insecure: false,
		},
		CustomEndpoint: &CustomEndpointConfig{
			Enabled: true,
			URL:     "https://metrics.company.com/ingest",
			Headers: map[string]string{
				"Authorization": "Bearer token",
				"Content-Type":  "application/json",
			},
			Format: "json",
		},
		TracingSampleRate: 0.25,
		DetailedMetrics:   true,
	}

	// Test Prometheus config
	assert.True(t, config.Prometheus.Enabled)
	assert.Equal(t, 9090, config.Prometheus.Port)
	assert.Equal(t, "/metrics", config.Prometheus.Path)
	assert.Equal(t, "ogem", config.Prometheus.Namespace)
	assert.Equal(t, "proxy", config.Prometheus.Subsystem)

	// Test Datadog config
	assert.True(t, config.Datadog.Enabled)
	assert.Equal(t, "test-dd-key", config.Datadog.APIKey)
	assert.Equal(t, "test-app-key", config.Datadog.AppKey)
	assert.Equal(t, "datadoghq.eu", config.Datadog.Site)
	assert.Equal(t, "ogem-proxy", config.Datadog.Service)
	assert.Equal(t, "production", config.Datadog.Env)
	assert.Equal(t, "1.2.3", config.Datadog.Version)
	assert.Contains(t, config.Datadog.Tags, "team:backend")
	assert.Contains(t, config.Datadog.Tags, "component:proxy")

	// Test OpenTelemetry config
	assert.True(t, config.OpenTelemetry.Enabled)
	assert.Equal(t, "https://api.honeycomb.io:443", config.OpenTelemetry.Endpoint)
	assert.Equal(t, "ogem-proxy", config.OpenTelemetry.ServiceName)
	assert.Equal(t, "1.2.3", config.OpenTelemetry.ServiceVersion)
	assert.Equal(t, "production", config.OpenTelemetry.Environment)
	assert.Equal(t, "test-key", config.OpenTelemetry.Headers["x-honeycomb-team"])
	assert.False(t, config.OpenTelemetry.Insecure)

	// Test Custom endpoint config
	assert.True(t, config.CustomEndpoint.Enabled)
	assert.Equal(t, "https://metrics.company.com/ingest", config.CustomEndpoint.URL)
	assert.Equal(t, "Bearer token", config.CustomEndpoint.Headers["Authorization"])
	assert.Equal(t, "application/json", config.CustomEndpoint.Headers["Content-Type"])
	assert.Equal(t, "json", config.CustomEndpoint.Format)

	// Test general config
	assert.Equal(t, 0.25, config.TracingSampleRate)
	assert.True(t, config.DetailedMetrics)
}