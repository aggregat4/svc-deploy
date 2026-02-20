// Package integrationtest provides health endpoint mocks for integration testing.
package integrationtest

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// HealthBehavior defines the behavior of the health mock server.
type HealthBehavior int

const (
	// HealthAlwaysHealthy always returns 200 OK immediately.
	HealthAlwaysHealthy HealthBehavior = iota
	// HealthAlwaysUnhealthy always returns 503 Service Unavailable.
	HealthAlwaysUnhealthy
	// HealthTimeout never responds (causes client timeout).
	HealthTimeout
	// HealthDelayedHealthy waits then returns 200 OK.
	HealthDelayedHealthy
	// HealthDelayedUnhealthy waits then returns 503.
	HealthDelayedUnhealthy
	// HealthFlapping alternates between healthy and unhealthy.
	HealthFlapping
)

// HealthServer mocks a health endpoint with configurable behaviors.
type HealthServer struct {
	mu         sync.RWMutex
	server     *httptest.Server
	behavior   HealthBehavior
	delay      time.Duration
	statusCode int
	response   string

	// For flapping behavior
	requestCount int
}

// NewHealthServer creates a new health endpoint mock server.
func NewHealthServer() *HealthServer {
	hs := &HealthServer{
		behavior:   HealthAlwaysHealthy,
		statusCode: http.StatusOK,
		response:   `{"status":"healthy"}`,
	}

	hs.server = httptest.NewServer(http.HandlerFunc(hs.handleRequest))

	return hs
}

// Close shuts down the test server.
func (hs *HealthServer) Close() {
	hs.server.Close()
}

// URL returns the health endpoint URL.
func (hs *HealthServer) URL() string {
	return hs.server.URL + "/healthz"
}

// SetBehavior sets the health behavior.
func (hs *HealthServer) SetBehavior(behavior HealthBehavior) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.behavior = behavior
	hs.requestCount = 0

	switch behavior {
	case HealthAlwaysHealthy, HealthDelayedHealthy:
		hs.statusCode = http.StatusOK
		hs.response = `{"status":"healthy"}`
	case HealthAlwaysUnhealthy, HealthDelayedUnhealthy:
		hs.statusCode = http.StatusServiceUnavailable
		hs.response = `{"status":"unhealthy"}`
	case HealthTimeout:
		// No response will be sent
	}
}

// SetDelay sets the delay for delayed behaviors.
func (hs *HealthServer) SetDelay(delay time.Duration) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.delay = delay
}

// SetCustomResponse sets a custom response body and status code.
func (hs *HealthServer) SetCustomResponse(statusCode int, response string) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.statusCode = statusCode
	hs.response = response
}

// GetRequestCount returns the number of health check requests received.
func (hs *HealthServer) GetRequestCount() int {
	hs.mu.RLock()
	defer hs.mu.RUnlock()
	return hs.requestCount
}

// handleRequest serves health check requests.
func (hs *HealthServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	hs.mu.Lock()
	defer hs.mu.Unlock()

	hs.requestCount++

	switch hs.behavior {
	case HealthTimeout:
		// Never respond - client will timeout
		select {}

	case HealthDelayedHealthy, HealthDelayedUnhealthy:
		time.Sleep(hs.delay)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(hs.statusCode)
		_, _ = w.Write([]byte(hs.response))

	case HealthFlapping:
		// Alternate between healthy and unhealthy
		if hs.requestCount%2 == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy"}`))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
		}

	default: // HealthAlwaysHealthy, HealthAlwaysUnhealthy
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(hs.statusCode)
		_, _ = w.Write([]byte(hs.response))
	}
}

// HealthyHealthServer is a convenience function for a healthy server.
func HealthyHealthServer() *HealthServer {
	hs := NewHealthServer()
	hs.SetBehavior(HealthAlwaysHealthy)
	return hs
}

// UnhealthyHealthServer is a convenience function for an unhealthy server.
func UnhealthyHealthServer() *HealthServer {
	hs := NewHealthServer()
	hs.SetBehavior(HealthAlwaysUnhealthy)
	return hs
}

// SlowHealthyHealthServer is a convenience function for a slow-but-healthy server.
func SlowHealthyHealthServer(delay time.Duration) *HealthServer {
	hs := NewHealthServer()
	hs.SetBehavior(HealthDelayedHealthy)
	hs.SetDelay(delay)
	return hs
}

// TimeoutHealthServer is a convenience function for a server that never responds.
func TimeoutHealthServer() *HealthServer {
	hs := NewHealthServer()
	hs.SetBehavior(HealthTimeout)
	return hs
}
