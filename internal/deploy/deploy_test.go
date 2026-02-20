package deploy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/testutil"
)

func TestDeployFresh(t *testing.T) {
	ctx := context.Background()

	// Setup mocks with post-extract callback
	fs := testutil.NewMockFS()
	fs.SetPostExtractCallback(func(dst string) {
		// Create the expected binary after extraction
		binaryPath := dst + "/bin/svc-a"
		fs.AddFile(binaryPath, []byte("binary"))
		// Create data directory for DB
		fs.AddDir(dst + "/data")
	})
	fetcher := testutil.NewMockArtifactFetcher()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	configRepo := testutil.NewMockConfigRepo()
	clock := testutil.NewMockClock(time.Now())

	// Setup service config
	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://github.com/org/svc-a/releases/download/{{.Version}}/{{.Artifact}}",
		ArtifactFilenameTemplate: "{{.Service}}-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		DBFilename:               "svc-a.db",
		StartupTimeout:           30,
		KeepReleases:             5,
	}

	// Add artifact and checksum
	artifactData := []byte("fake tarball content")
	fetcher.AddArtifact(
		"https://github.com/org/svc-a/releases/download/v1.0.0/svc-a-v1.0.0.tar.gz",
		artifactData,
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	)

	// Setup health check
	healthChecker.SetHealthy("http://127.0.0.1:8080/healthz", true)

	deps := Deps{
		FS:            fs,
		Fetcher:       fetcher,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		ConfigRepo:    configRepo,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.0.0", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Version != "v1.0.0" {
		t.Errorf("Version = %q, want %q", result.Version, "v1.0.0")
	}

	if result.PreviousVersion != "" {
		t.Errorf("PreviousVersion should be empty for fresh deploy, got %q", result.PreviousVersion)
	}

	// Verify symlinks were updated
	current, _ := symlinkMgr.GetCurrent("/opt/a4-services/svc-a")
	if current != "v1.0.0" {
		t.Errorf("current = %q, want %q", current, "v1.0.0")
	}
}

func TestDeployUpgrade(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	fs := testutil.NewMockFS()
	fs.SetPostExtractCallback(func(dst string) {
		// Create the expected binary after extraction
		binaryPath := dst + "/bin/svc-a"
		fs.AddFile(binaryPath, []byte("new binary"))
		// Create data directory for DB
		fs.AddDir(dst + "/data")
	})
	fetcher := testutil.NewMockArtifactFetcher()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	configRepo := testutil.NewMockConfigRepo()
	clock := testutil.NewMockClock(time.Now())

	// Setup existing release
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("old binary"))
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/data/svc-a.db", []byte("old db"))
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	// Setup service config
	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://github.com/org/svc-a/releases/download/{{.Version}}/{{.Artifact}}",
		ArtifactFilenameTemplate: "{{.Service}}-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		DBFilename:               "svc-a.db",
		StartupTimeout:           30,
		KeepReleases:             5,
	}

	// Add new artifact
	fetcher.AddArtifact(
		"https://github.com/org/svc-a/releases/download/v1.1.0/svc-a-v1.1.0.tar.gz",
		[]byte("new tarball"),
		"sha256hash",
	)

	healthChecker.SetHealthy("http://127.0.0.1:8080/healthz", true)

	deps := Deps{
		FS:            fs,
		Fetcher:       fetcher,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		ConfigRepo:    configRepo,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.1.0", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Version != "v1.1.0" {
		t.Errorf("Version = %q, want %q", result.Version, "v1.1.0")
	}

	if result.PreviousVersion != "v1.0.0" {
		t.Errorf("PreviousVersion = %q, want %q", result.PreviousVersion, "v1.0.0")
	}

	// Verify current was updated
	current, _ := symlinkMgr.GetCurrent("/opt/a4-services/svc-a")
	if current != "v1.1.0" {
		t.Errorf("current = %q, want %q", current, "v1.1.0")
	}

	// Verify previous was updated
	previous, _ := symlinkMgr.GetPrevious("/opt/a4-services/svc-a")
	if previous != "v1.0.0" {
		t.Errorf("previous = %q, want %q", previous, "v1.0.0")
	}
}

func TestDeployHealthCheckFailure(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	fs := testutil.NewMockFS()
	fetcher := testutil.NewMockArtifactFetcher()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	configRepo := testutil.NewMockConfigRepo()
	clock := testutil.NewMockClock(time.Now())

	// Setup existing release
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("old binary"))
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://github.com/org/svc-a/releases/download/{{.Version}}/{{.Service}}-{{.Version}}.tar.gz",
		ArtifactFilenameTemplate: "{{.Service}}-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		StartupTimeout:           1,
		KeepReleases:             5,
	}

	fetcher.AddArtifact(
		"https://github.com/org/svc-a/releases/download/v1.1.0/svc-a-v1.1.0.tar.gz",
		[]byte("new tarball"),
		"sha256hash",
	)

	// Health check will fail
	healthChecker.SetHealthy("http://127.0.0.1:8080/healthz", false)

	deps := Deps{
		FS:            fs,
		Fetcher:       fetcher,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		ConfigRepo:    configRepo,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.1.0", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to health check failure")
	}

	// Verify rollback occurred - current should be back to v1.0.0
	current, _ := symlinkMgr.GetCurrent("/opt/a4-services/svc-a")
	if current != "v1.0.0" {
		t.Errorf("after rollback, current = %q, want %q", current, "v1.0.0")
	}
}

func TestDeployChecksumMismatch(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	fetcher := testutil.NewMockArtifactFetcher()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	configRepo := testutil.NewMockConfigRepo()
	clock := testutil.NewMockClock(time.Now())

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://github.com/org/svc-a/releases/download/{{.Version}}/{{.Service}}-{{.Version}}.tar.gz",
		ArtifactFilenameTemplate: "{{.Service}}-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		StartupTimeout:           30,
		KeepReleases:             5,
	}

	// Set up fetcher to return checksum error
	fetcher.SetError(fmt.Errorf("checksum mismatch"))

	deps := Deps{
		FS:            fs,
		Fetcher:       fetcher,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		ConfigRepo:    configRepo,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.0.0", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to checksum mismatch")
	}

	if err.Error() != "fetching artifact: checksum mismatch" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDeployLockAcquisition(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	fetcher := testutil.NewMockArtifactFetcher()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	configRepo := testutil.NewMockConfigRepo()
	clock := testutil.NewMockClock(time.Now())

	// First acquire the lock
	release, err := locker.Acquire("svc-a")
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer release()

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/app",
		HealthCheckURL:           "http://localhost:8080/healthz",
		SystemdUnit:              "app.service",
		StartupTimeout:           30,
		KeepReleases:             5,
	}

	deps := Deps{
		FS:            fs,
		Fetcher:       fetcher,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		ConfigRepo:    configRepo,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.0.0", deps)
	_, err = op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to lock already held")
	}

	if err.Error() != "acquiring lock: already locked" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDeployPrunesOldReleases(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	fs := testutil.NewMockFS()
	fs.SetPostExtractCallback(func(dst string) {
		binaryPath := dst + "/bin/svc-a"
		fs.AddFile(binaryPath, []byte("binary"))
		fs.AddDir(dst + "/data")
	})
	fetcher := testutil.NewMockArtifactFetcher()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	configRepo := testutil.NewMockConfigRepo()
	clock := testutil.NewMockClock(time.Now())

	// Setup existing releases (more than keep limit)
	fs.AddDir("/opt/a4-services/svc-a/releases")
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("old binary"))
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.1.0/bin/svc-a", []byte("old binary"))
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.2.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.2.0/bin/svc-a", []byte("old binary"))
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.3.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.3.0/bin/svc-a", []byte("old binary"))
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.4.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.4.0/bin/svc-a", []byte("old binary"))

	// Current is v1.4.0, previous is v1.3.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.4.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.3.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.4.0")
	fs.AddSymlink("/opt/a4-services/svc-a/previous", "/opt/a4-services/svc-a/releases/v1.3.0")

	// Deploy new version
	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://github.com/org/svc-a/releases/download/{{.Version}}/{{.Artifact}}",
		ArtifactFilenameTemplate: "{{.Service}}-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		DBFilename:               "svc-a.db",
		StartupTimeout:           30,
		KeepReleases:             3, // Keep 3 releases
	}

	fetcher.AddArtifact(
		"https://github.com/org/svc-a/releases/download/v1.5.0/svc-a-v1.5.0.tar.gz",
		[]byte("new tarball"),
		"sha256hash",
	)

	healthChecker.SetHealthy("http://127.0.0.1:8080/healthz", true)

	deps := Deps{
		FS:            fs,
		Fetcher:       fetcher,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		ConfigRepo:    configRepo,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.5.0", deps)
	_, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After deploy, old releases should be pruned
	// We had: v1.0.0, v1.1.0, v1.2.0, v1.3.0, v1.4.0 (current), + v1.5.0 (new current)
	// Protected: v1.4.0 (previous), v1.5.0 (current)
	// Non-protected: v1.0.0, v1.1.0, v1.2.0, v1.3.0
	// Keep 3 newest non-protected: v1.3.0, v1.2.0, v1.1.0
	// Removed: v1.0.0

	// Check that oldest release was removed
	if fs.Exists("/opt/a4-services/svc-a/releases/v1.0.0") {
		t.Error("v1.0.0 should have been pruned")
	}

	// Check that kept non-protected releases still exist
	if !fs.Exists("/opt/a4-services/svc-a/releases/v1.1.0") {
		t.Error("v1.1.0 should still exist (within keep limit)")
	}
	if !fs.Exists("/opt/a4-services/svc-a/releases/v1.2.0") {
		t.Error("v1.2.0 should still exist (within keep limit)")
	}
	if !fs.Exists("/opt/a4-services/svc-a/releases/v1.3.0") {
		t.Error("v1.3.0 should still exist (within keep limit)")
	}

	// Check that protected releases still exist
	if !fs.Exists("/opt/a4-services/svc-a/releases/v1.4.0") {
		t.Error("v1.4.0 should still exist (previous)")
	}
	if !fs.Exists("/opt/a4-services/svc-a/releases/v1.5.0") {
		t.Error("v1.5.0 should still exist (current)")
	}
}
