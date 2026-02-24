// Package testutil provides mock implementations for testing.
package testutil

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/a4/svc-deploy/internal/interfaces"
)

// MockFS implements interfaces.FS for testing.
type MockFS struct {
	mu                  sync.RWMutex
	files               map[string][]byte
	dirs                map[string]bool
	symlinks            map[string]string
	fileInfo            map[string]interfaces.FileInfo
	postExtractCallback func(string)
	diskFree            map[string]uint64
}

// NewMockFS creates a new mock filesystem.
func NewMockFS() *MockFS {
	return &MockFS{
		files:    make(map[string][]byte),
		dirs:     make(map[string]bool),
		symlinks: make(map[string]string),
		fileInfo: make(map[string]interfaces.FileInfo),
	}
}

func (m *MockFS) Exists(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[path]
	if ok {
		return true
	}
	_, ok = m.dirs[path]
	if ok {
		return true
	}
	_, ok = m.symlinks[path]
	return ok
}

func (m *MockFS) MkdirAll(path string, perm int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs[path] = true
	m.fileInfo[path] = interfaces.FileInfo{
		Name:    filepath.Base(path),
		IsDir:   true,
		Mode:    perm,
		ModTime: time.Now(),
	}
	return nil
}

func (m *MockFS) Remove(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, path)
	delete(m.dirs, path)
	delete(m.symlinks, path)
	return nil
}

func (m *MockFS) RemoveAll(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.files {
		if strings.HasPrefix(k, path) {
			delete(m.files, k)
			delete(m.fileInfo, k)
		}
	}
	for k := range m.dirs {
		if strings.HasPrefix(k, path) {
			delete(m.dirs, k)
			delete(m.fileInfo, k)
		}
	}
	for k := range m.symlinks {
		if strings.HasPrefix(k, path) {
			delete(m.symlinks, k)
		}
	}
	return nil
}

func (m *MockFS) CopyFile(src, dst string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[src]
	if !ok {
		return fmt.Errorf("source file not found: %s", src)
	}
	m.files[dst] = data
	m.fileInfo[dst] = m.fileInfo[src]
	return nil
}

func (m *MockFS) CopyDir(src, dst string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.files {
		if strings.HasPrefix(k, src+"/") {
			rel := strings.TrimPrefix(k, src+"/")
			m.files[dst+"/"+rel] = v
		}
	}
	return nil
}

func (m *MockFS) ReadFile(path string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.files[path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", path)
	}
	return data, nil
}

func (m *MockFS) WriteFile(path string, data []byte, perm int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = data
	m.fileInfo[path] = interfaces.FileInfo{
		Name:    filepath.Base(path),
		Size:    int64(len(data)),
		Mode:    perm,
		ModTime: time.Now(),
	}
	return nil
}

func (m *MockFS) CreateCompressedBackup(src, dst string) error {
	// Mock: just copy the file
	return m.CopyFile(src, dst)
}

func (m *MockFS) ExtractTar(r io.Reader, dst string) error {
	// Mock implementation: read the data and create a basic structure
	// In real integration tests, use actual tar data
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a marker file to indicate extraction happened
	m.dirs[dst] = true
	m.fileInfo[dst] = interfaces.FileInfo{
		Name:    dst,
		IsDir:   true,
		Mode:    0755,
		ModTime: time.Now(),
	}

	// Store the extracted data so tests can verify it
	m.files[dst+"/.extracted"] = data

	// Call post-extract callback if set
	if m.postExtractCallback != nil {
		m.mu.Unlock()
		m.postExtractCallback(dst)
		m.mu.Lock()
	}

	return nil
}

// SetPostExtractCallback sets a callback to be called after ExtractTar.
func (m *MockFS) SetPostExtractCallback(fn func(string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.postExtractCallback = fn
}

func (m *MockFS) ListDirectory(path string) ([]interfaces.DirEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make(map[string]interfaces.DirEntry)
	prefix := path + "/"

	for k := range m.dirs {
		if strings.HasPrefix(k, prefix) {
			rel := strings.TrimPrefix(k, prefix)
			if !strings.Contains(rel, "/") {
				entries[rel] = interfaces.DirEntry{Name: rel, IsDir: true}
			}
		}
	}

	for k := range m.files {
		if strings.HasPrefix(k, prefix) {
			rel := strings.TrimPrefix(k, prefix)
			if !strings.Contains(rel, "/") {
				entries[rel] = interfaces.DirEntry{Name: rel, IsDir: false}
			}
		}
	}

	result := make([]interfaces.DirEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}
	return result, nil
}

func (m *MockFS) Stat(path string) (interfaces.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, ok := m.fileInfo[path]
	if !ok {
		return interfaces.FileInfo{}, fmt.Errorf("file not found: %s", path)
	}
	return info, nil
}

func (m *MockFS) Symlink(oldname, newname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.symlinks[newname] = oldname
	return nil
}

func (m *MockFS) Readlink(path string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	target, ok := m.symlinks[path]
	if !ok {
		return "", fmt.Errorf("symlink not found: %s", path)
	}
	return target, nil
}

func (m *MockFS) DiskFree(path string) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if free, ok := m.diskFree[path]; ok {
		return free, nil
	}
	return 10 * 1024 * 1024 * 1024, nil // 10GB free default
}

// SetDiskFree sets the available disk space for a path (test helper).
func (m *MockFS) SetDiskFree(path string, bytes uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.diskFree == nil {
		m.diskFree = make(map[string]uint64)
	}
	m.diskFree[path] = bytes
}

// AddFile adds a file to the mock filesystem (test helper).
func (m *MockFS) AddFile(path string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = data
	m.fileInfo[path] = interfaces.FileInfo{
		Name:    filepath.Base(path),
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}
}

// AddSymlink adds a symlink to the mock filesystem (test helper).
func (m *MockFS) AddSymlink(name, target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.symlinks[name] = target
}

// AddDir adds a directory to the mock filesystem (test helper).
func (m *MockFS) AddDir(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs[path] = true
	m.fileInfo[path] = interfaces.FileInfo{
		Name:    filepath.Base(path),
		IsDir:   true,
		Mode:    0755,
		ModTime: time.Now(),
	}
}

// MockArtifactFetcher implements interfaces.ArtifactFetcher for testing.
type MockArtifactFetcher struct {
	mu        sync.Mutex
	artifacts map[string][]byte
	checksums map[string]string
	shouldErr error
}

// NewMockArtifactFetcher creates a new mock artifact fetcher.
func NewMockArtifactFetcher() *MockArtifactFetcher {
	return &MockArtifactFetcher{
		artifacts: make(map[string][]byte),
		checksums: make(map[string]string),
	}
}

func (m *MockArtifactFetcher) Fetch(ctx context.Context, url string, checksumURL string) (io.ReadCloser, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldErr != nil {
		return nil, "", m.shouldErr
	}

	data, ok := m.artifacts[url]
	if !ok {
		return nil, "", fmt.Errorf("artifact not found: %s", url)
	}

	checksum, ok := m.checksums[checksumURL]
	if !ok {
		return nil, "", fmt.Errorf("checksum not found: %s", checksumURL)
	}

	return io.NopCloser(strings.NewReader(string(data))), checksum, nil
}

// AddArtifact adds an artifact for testing.
func (m *MockArtifactFetcher) AddArtifact(url string, data []byte, checksum string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.artifacts[url] = data
	m.checksums[url+".sha256"] = checksum
}

// SetError sets an error to return.
func (m *MockArtifactFetcher) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldErr = err
}

// MockServiceManager implements interfaces.ServiceManager for testing.
type MockServiceManager struct {
	mu       sync.Mutex
	status   map[string]interfaces.ServiceStatus
	errors   map[string]error
	restarts []string
	stops    []string
}

// NewMockServiceManager creates a new mock service manager.
func NewMockServiceManager() *MockServiceManager {
	return &MockServiceManager{
		status:   make(map[string]interfaces.ServiceStatus),
		errors:   make(map[string]error),
		restarts: []string{},
		stops:    []string{},
	}
}

func (m *MockServiceManager) Restart(ctx context.Context, unit string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restarts = append(m.restarts, unit)
	if err, ok := m.errors[unit]; ok {
		return err
	}
	m.status[unit] = interfaces.ServiceStatus{
		Active:    true,
		Loaded:    true,
		Unit:      unit,
		SubStatus: "running",
	}
	return nil
}

func (m *MockServiceManager) Stop(ctx context.Context, unit string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stops = append(m.stops, unit)
	m.status[unit] = interfaces.ServiceStatus{
		Active:    false,
		Loaded:    true,
		Unit:      unit,
		SubStatus: "dead",
	}
	return nil
}

func (m *MockServiceManager) Status(ctx context.Context, unit string) (interfaces.ServiceStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.status[unit]
	if !ok {
		return interfaces.ServiceStatus{}, fmt.Errorf("unit not found: %s", unit)
	}
	return status, nil
}

// SetStatus sets the status for a unit.
func (m *MockServiceManager) SetStatus(unit string, status interfaces.ServiceStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status[unit] = status
}

// SetError sets an error for a unit operation.
func (m *MockServiceManager) SetError(unit string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[unit] = err
}

// GetRestarts returns the list of restarted units.
func (m *MockServiceManager) GetRestarts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.restarts...)
}

// WasRestartCalled returns true if the given unit was restarted.
func (m *MockServiceManager) WasRestartCalled(unit string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.restarts {
		if u == unit {
			return true
		}
	}
	return false
}

// SetRestartSuccess configures the mock to succeed for restarts.
func (m *MockServiceManager) SetRestartSuccess(success bool) {
	// This method exists for API compatibility; actual behavior controlled by SetError
	m.mu.Lock()
	defer m.mu.Unlock()
	if success {
		// Clear any errors to allow success
		m.errors = make(map[string]error)
	}
}

// MockHealthChecker implements interfaces.HealthChecker for testing.
type MockHealthChecker struct {
	mu      sync.Mutex
	healthy map[string]bool
	latency map[string]time.Duration
}

// NewMockHealthChecker creates a new mock health checker.
func NewMockHealthChecker() *MockHealthChecker {
	return &MockHealthChecker{
		healthy: make(map[string]bool),
		latency: make(map[string]time.Duration),
	}
}

func (m *MockHealthChecker) Check(ctx context.Context, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if latency, ok := m.latency[url]; ok && latency > 0 {
		select {
		case <-time.After(latency):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if healthy, ok := m.healthy[url]; ok && healthy {
		return nil
	}
	return fmt.Errorf("unhealthy")
}

// SetHealthy sets the health status for a URL.
func (m *MockHealthChecker) SetHealthy(url string, healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy[url] = healthy
}

// SetLatency sets the latency for a URL.
func (m *MockHealthChecker) SetLatency(url string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency[url] = d
}

// MockLocker implements interfaces.Locker for testing.
type MockLocker struct {
	mu    sync.Mutex
	locks map[string]bool
}

// NewMockLocker creates a new mock locker.
func NewMockLocker() *MockLocker {
	return &MockLocker{
		locks: make(map[string]bool),
	}
}

func (m *MockLocker) Acquire(service string) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.locks[service] {
		return nil, fmt.Errorf("already locked")
	}

	m.locks[service] = true
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.locks, service)
	}, nil
}

// MockSymlinkManager implements interfaces.SymlinkManager for testing.
type MockSymlinkManager struct {
	mu       sync.Mutex
	current  map[string]string
	previous map[string]string
}

// NewMockSymlinkManager creates a new mock symlink manager.
func NewMockSymlinkManager() *MockSymlinkManager {
	return &MockSymlinkManager{
		current:  make(map[string]string),
		previous: make(map[string]string),
	}
}

func (m *MockSymlinkManager) GetCurrent(servicePath string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.current[servicePath]
	if !ok {
		return "", fmt.Errorf("no current symlink")
	}
	return filepath.Base(current), nil
}

func (m *MockSymlinkManager) GetPrevious(servicePath string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	previous, ok := m.previous[servicePath]
	if !ok {
		return "", fmt.Errorf("no previous symlink")
	}
	return filepath.Base(previous), nil
}

func (m *MockSymlinkManager) SetCurrent(servicePath, releaseVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	releasePath := filepath.Join(servicePath, "releases", releaseVersion)

	// Move current to previous
	if current, ok := m.current[servicePath]; ok {
		m.previous[servicePath] = current
	}

	m.current[servicePath] = releasePath
	return nil
}

func (m *MockSymlinkManager) RollbackCurrent(servicePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	previous, ok := m.previous[servicePath]
	if !ok {
		return fmt.Errorf("no previous to rollback to")
	}

	current, ok := m.current[servicePath]
	if ok {
		m.previous[servicePath] = current
	}

	m.current[servicePath] = previous
	return nil
}

// SetCurrent sets the current symlink directly (test helper).
func (m *MockSymlinkManager) SetCurrentDirect(servicePath, target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current[servicePath] = target
}

// SetPrevious sets the previous symlink directly (test helper).
func (m *MockSymlinkManager) SetPreviousDirect(servicePath, target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.previous[servicePath] = target
}

// MockConfigRepo implements interfaces.ConfigRepo for testing.
type MockConfigRepo struct {
	mu             sync.Mutex
	commit         string
	runtimeConfigs map[string][]byte
}

// NewMockConfigRepo creates a new mock config repo.
func NewMockConfigRepo() *MockConfigRepo {
	return &MockConfigRepo{
		commit:         "abc123",
		runtimeConfigs: make(map[string][]byte),
	}
}

func (m *MockConfigRepo) GetCurrentCommit() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.commit, nil
}

func (m *MockConfigRepo) GetRuntimeConfig(service string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	config, ok := m.runtimeConfigs[service]
	if !ok {
		return nil, fmt.Errorf("no runtime config for %s", service)
	}
	return config, nil
}

// SetCommit sets the commit hash.
func (m *MockConfigRepo) SetCommit(commit string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commit = commit
}

// SetRuntimeConfig sets the runtime config for a service.
func (m *MockConfigRepo) SetRuntimeConfig(service string, config []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtimeConfigs[service] = config
}

// MockClock implements interfaces.Clock for testing.
type MockClock struct {
	mu      sync.Mutex
	now     time.Time
	sinceFn func(time.Time) time.Duration
}

// NewMockClock creates a new mock clock.
func NewMockClock(now time.Time) *MockClock {
	return &MockClock{now: now}
}

func (m *MockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *MockClock) Since(t time.Time) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sinceFn != nil {
		return m.sinceFn(t)
	}
	return m.now.Sub(t)
}

// SetNow sets the current time.
func (m *MockClock) SetNow(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = t
}

// Advance advances the clock by the given duration.
func (m *MockClock) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}
