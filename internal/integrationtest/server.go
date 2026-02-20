// Package integrationtest provides HTTP test servers for integration testing.
package integrationtest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
)

// ArtifactServer serves artifacts and their checksums via HTTP for testing.
// It uses httptest.Server to avoid external network dependencies.
type ArtifactServer struct {
	mu        sync.RWMutex
	server    *httptest.Server
	artifacts map[string][]byte // URL path -> content
	checksums map[string]string // URL path -> checksum
	baseURL   string
}

// NewArtifactServer creates a new artifact server for integration testing.
func NewArtifactServer() *ArtifactServer {
	s := &ArtifactServer{
		artifacts: make(map[string][]byte),
		checksums: make(map[string]string),
	}

	s.server = httptest.NewServer(http.HandlerFunc(s.handleRequest))
	s.baseURL = s.server.URL

	return s
}

// Close shuts down the test server.
func (s *ArtifactServer) Close() {
	s.server.Close()
}

// URL returns the base URL of the server.
func (s *ArtifactServer) URL() string {
	return s.baseURL
}

// AddArtifact registers an artifact to be served.
// artifactPath should be the URL path (e.g., "/releases/v1.0.0/app.tar.gz")
func (s *ArtifactServer) AddArtifact(artifactPath string, data []byte, checksum string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.artifacts[artifactPath] = data
	s.checksums[artifactPath] = checksum
}

// AddArtifactFromFixture adds an artifact from a FixtureSet.
// Returns the artifact URL and checksum URL.
func (s *ArtifactServer) AddArtifactFromFixture(fs *FixtureSet, version string) (artifactURL, checksumURL string, err error) {
	artifact, ok := fs.Artifacts[version]
	if !ok {
		return "", "", fmt.Errorf("version %s not found in fixture set", version)
	}

	filename := fmt.Sprintf("%s-%s.tar.gz", fs.Service, version)
	artifactPath := path.Join("/releases", version, filename)
	checksumPath := artifactPath + ".sha256"

	s.mu.Lock()
	defer s.mu.Unlock()

	s.artifacts[artifactPath] = artifact.Content

	// Create checksum content in GNU format
	checksumContent := fmt.Sprintf("%s  %s\n", artifact.Checksum, filename)
	s.checksums[artifactPath] = checksumContent

	artifactURL = s.baseURL + artifactPath
	checksumURL = s.baseURL + checksumPath

	return artifactURL, checksumURL, nil
}

// handleRequest serves artifact and checksum requests.
func (s *ArtifactServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	requestPath := r.URL.Path

	// Check if this is a checksum request
	if strings.HasSuffix(requestPath, ".sha256") {
		artifactPath := strings.TrimSuffix(requestPath, ".sha256")
		checksum, ok := s.checksums[artifactPath]
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(checksum))
		return
	}

	// This is an artifact request
	data, ok := s.artifacts[requestPath]
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GetArtifactURL returns the full URL for an artifact.
func (s *ArtifactServer) GetArtifactURL(artifactPath string) string {
	return s.baseURL + artifactPath
}

// GetChecksumURL returns the full URL for a checksum.
func (s *ArtifactServer) GetChecksumURL(artifactPath string) string {
	return s.baseURL + artifactPath + ".sha256"
}

// RequestRecorder records all requests made to the server for verification.
type RequestRecorder struct {
	mu       sync.RWMutex
	requests []RequestRecord
}

// RequestRecord represents a single HTTP request.
type RequestRecord struct {
	Method string
	Path   string
}

// NewRequestRecorder creates a new request recorder.
func NewRequestRecorder() *RequestRecorder {
	return &RequestRecorder{
		requests: make([]RequestRecord, 0),
	}
}

// Record records a request.
func (r *RequestRecorder) Record(method, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, RequestRecord{Method: method, Path: path})
}

// GetRequests returns all recorded requests.
func (r *RequestRecorder) GetRequests() []RequestRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy
	result := make([]RequestRecord, len(r.requests))
	copy(result, r.requests)
	return result
}

// Clear clears all recorded requests.
func (r *RequestRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = r.requests[:0]
}

// ArtifactServerWithRecorder is an artifact server that records all requests.
type ArtifactServerWithRecorder struct {
	*ArtifactServer
	recorder *RequestRecorder
}

// NewArtifactServerWithRecorder creates an artifact server that records requests.
func NewArtifactServerWithRecorder() *ArtifactServerWithRecorder {
	s := &ArtifactServerWithRecorder{
		recorder: NewRequestRecorder(),
	}

	s.ArtifactServer = &ArtifactServer{
		artifacts: make(map[string][]byte),
		checksums: make(map[string]string),
	}

	s.server = httptest.NewServer(http.HandlerFunc(s.handleRequestWithRecorder))
	s.baseURL = s.server.URL

	return s
}

func (s *ArtifactServerWithRecorder) handleRequestWithRecorder(w http.ResponseWriter, r *http.Request) {
	s.recorder.Record(r.Method, r.URL.Path)
	s.handleRequest(w, r)
}

// GetRecorder returns the request recorder.
func (s *ArtifactServerWithRecorder) GetRecorder() *RequestRecorder {
	return s.recorder
}
