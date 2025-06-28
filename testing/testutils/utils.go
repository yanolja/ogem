package testutils

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestLogger creates a test logger that captures logs for verification
func TestLogger(t *testing.T) *zap.Logger {
	return zaptest.NewLogger(t)
}

// TestSugaredLogger creates a sugared test logger
func TestSugaredLogger(t *testing.T) *zap.SugaredLogger {
	return zaptest.NewLogger(t).Sugar()
}

// TestContext creates a context with timeout for tests
func TestContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

// TestServer creates a test HTTP server with the given handler
func TestServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(handler)
}

// MockHTTPResponse creates a mock HTTP response
func MockHTTPResponse(statusCode int, body interface{}) *http.Response {
	var bodyReader io.Reader
	
	if body != nil {
		if str, ok := body.(string); ok {
			bodyReader = bytes.NewBufferString(str)
		} else {
			bodyBytes, _ := json.Marshal(body)
			bodyReader = bytes.NewBuffer(bodyBytes)
		}
	} else {
		bodyReader = bytes.NewBufferString("")
	}

	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bodyReader),
	}
}

// AssertJSONEqual compares two JSON objects for equality
func AssertJSONEqual(t *testing.T, expected, actual interface{}) {
	expectedJSON, err := json.Marshal(expected)
	require.NoError(t, err)
	
	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	
	assert.JSONEq(t, string(expectedJSON), string(actualJSON))
}

// AssertContainsSubstring checks if a string contains a substring
func AssertContainsSubstring(t *testing.T, haystack, needle string, msgAndArgs ...interface{}) {
	assert.Contains(t, haystack, needle, msgAndArgs...)
}

// MockTimeNow provides a controllable time for testing
type MockTime struct {
	currentTime time.Time
}

func NewMockTime(t time.Time) *MockTime {
	return &MockTime{currentTime: t}
}

func (m *MockTime) Now() time.Time {
	return m.currentTime
}

func (m *MockTime) Advance(d time.Duration) {
	m.currentTime = m.currentTime.Add(d)
}

// TestConfig provides common test configuration
type TestConfig struct {
	BaseURL    string
	APIKey     string
	TenantID   string
	Debug      bool
	Timeout    time.Duration
}

func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		BaseURL:  "http://localhost:8080",
		APIKey:   "test-api-key",
		TenantID: "test-tenant",
		Debug:    true,
		Timeout:  30 * time.Second,
	}
}

// TestMetrics provides utilities for testing metrics
type TestMetrics struct {
	counters map[string]int
	gauges   map[string]float64
	timers   map[string][]time.Duration
}

func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		counters: make(map[string]int),
		gauges:   make(map[string]float64),
		timers:   make(map[string][]time.Duration),
	}
}

func (tm *TestMetrics) IncrementCounter(name string, delta int) {
	tm.counters[name] += delta
}

func (tm *TestMetrics) SetGauge(name string, value float64) {
	tm.gauges[name] = value
}

func (tm *TestMetrics) RecordTimer(name string, duration time.Duration) {
	tm.timers[name] = append(tm.timers[name], duration)
}

func (tm *TestMetrics) GetCounter(name string) int {
	return tm.counters[name]
}

func (tm *TestMetrics) GetGauge(name string) float64 {
	return tm.gauges[name]
}

func (tm *TestMetrics) GetTimerCount(name string) int {
	return len(tm.timers[name])
}

// TestData provides common test data structures
type TestData struct {
	Messages []TestMessage `json:"messages"`
	Model    string        `json:"model"`
	Settings TestSettings  `json:"settings"`
}

type TestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TestSettings struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"max_tokens,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
}

func DefaultTestData() *TestData {
	temp := 0.7
	maxTokens := 100
	topP := 0.9
	
	return &TestData{
		Messages: []TestMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello, world!"},
		},
		Model: "gpt-3.5-turbo",
		Settings: TestSettings{
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			TopP:        &topP,
		},
	}
}

// WaitForCondition waits for a condition to be true or timeout
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Condition not met within timeout: %s", message)
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}