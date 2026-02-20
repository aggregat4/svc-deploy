package rollback

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/testutil"
)

func TestRollbackToPrevious(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Setup existing releases
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("binary"))
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/data/svc-a.db", []byte("db"))

	fs.AddDir("/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.1.0/bin/svc-a", []byte("binary"))
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.1.0/data/svc-a.db", []byte("db"))

	// Current is v1.1.0, previous is v1.0.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		DBFilename:               "svc-a.db",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	healthChecker.SetHealthy("http://127.0.0.1:8080/healthz", true)

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	// Rollback without explicit target - should use previous
	op := New(svcCfg, "svc-a", "", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Version != "v1.0.0" {
		t.Errorf("Version = %q, want %q", result.Version, "v1.0.0")
	}

	if result.PreviousVersion != "v1.1.0" {
		t.Errorf("PreviousVersion = %q, want %q", result.PreviousVersion, "v1.1.0")
	}

	// Verify current is now v1.0.0
	current, _ := symlinkMgr.GetCurrent("/opt/a4-services/svc-a")
	if current != "v1.0.0" {
		t.Errorf("current = %q, want %q", current, "v1.0.0")
	}
}

func TestRollbackToSpecificVersion(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Setup existing releases
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("binary"))
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/data/svc-a.db", []byte("db"))

	fs.AddDir("/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.1.0/bin/svc-a", []byte("binary"))
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.1.0/data/svc-a.db", []byte("db"))

	fs.AddDir("/opt/a4-services/svc-a/releases/v1.2.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.2.0/bin/svc-a", []byte("binary"))
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.2.0/data/svc-a.db", []byte("db"))

	// Current is v1.2.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.2.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		DBFilename:               "svc-a.db",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	healthChecker.SetHealthy("http://127.0.0.1:8080/healthz", true)

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	// Rollback to specific version v1.0.0
	op := New(svcCfg, "svc-a", "v1.0.0", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Version != "v1.0.0" {
		t.Errorf("Version = %q, want %q", result.Version, "v1.0.0")
	}

	// Verify current is now v1.0.0
	current, _ := symlinkMgr.GetCurrent("/opt/a4-services/svc-a")
	if current != "v1.0.0" {
		t.Errorf("current = %q, want %q", current, "v1.0.0")
	}
}

func TestRollbackToMissingVersion(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Current is v1.1.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		DBFilename:               "svc-a.db",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	// Try to rollback to non-existent version
	op := New(svcCfg, "svc-a", "v0.9.0", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to missing version")
	}

	expectedErr := "target release v0.9.0 does not exist"
	if err.Error() != expectedErr {
		t.Errorf("error = %q, want %q", err.Error(), expectedErr)
	}
}

func TestRollbackHealthCheckFailure(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Setup releases
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("binary"))

	fs.AddDir("/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.1.0/bin/svc-a", []byte("binary"))

	// Current is v1.1.0, previous is v1.0.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		RollbackTimeout:          1,
		KeepReleases:             5,
	}

	// Health check will fail
	healthChecker.SetHealthy("http://127.0.0.1:8080/healthz", false)

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to health check failure")
	}

	// Verify we restored to prior current (v1.1.0)
	current, _ := symlinkMgr.GetCurrent("/opt/a4-services/svc-a")
	if current != "v1.1.0" {
		t.Errorf("after failed rollback, current = %q, want %q", current, "v1.1.0")
	}
}

func TestRollbackMissingBinary(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Setup release without binary
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	// No binary!

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.0.0", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to missing binary")
	}

	expectedMsg := "binary not found in target release"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("error = %q, expected to contain %q", err.Error(), expectedMsg)
	}
}

func TestRollbackMissingDB(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Setup release with binary but no DB
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("binary"))
	// No DB!

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		DBFilename:               "svc-a.db",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "v1.0.0", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to missing DB")
	}

	expectedMsg := "database not found in target release"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("error = %q, expected to contain %q", err.Error(), expectedMsg)
	}
}

func TestRollbackNoPrevious(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Current exists but no previous
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")
	// No previous set

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	// Try rollback without explicit target and no previous
	op := New(svcCfg, "svc-a", "", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to no previous release")
	}

	expectedMsg := "no previous release to rollback to"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("error = %q, expected to contain %q", err.Error(), expectedMsg)
	}
}

func TestRollbackLockAcquisition(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
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
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
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

func TestRollbackServiceRestartFailure(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	locker := testutil.NewMockLocker()
	svcMgr := testutil.NewMockServiceManager()
	healthChecker := testutil.NewMockHealthChecker()
	symlinkMgr := testutil.NewMockSymlinkManager()
	clock := testutil.NewMockClock(time.Now())

	// Setup releases
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/bin/svc-a", []byte("binary"))

	fs.AddDir("/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.1.0/bin/svc-a", []byte("binary"))

	// Current is v1.1.0, previous is v1.0.0
	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcCfg := config.ServiceConfig{
		ReleaseURLTemplate:       "https://example.com/{{.Version}}",
		ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
		BinaryPath:               "bin/svc-a",
		HealthCheckURL:           "http://127.0.0.1:8080/healthz",
		SystemdUnit:              "svc-a.service",
		RollbackTimeout:          30,
		KeepReleases:             5,
	}

	// Simulate restart failure
	svcMgr.SetError("svc-a.service", fmt.Errorf("systemctl failed"))

	deps := Deps{
		FS:            fs,
		Locker:        locker,
		ServiceMgr:    svcMgr,
		HealthChecker: healthChecker,
		SymlinkMgr:    symlinkMgr,
		Clock:         clock,
	}

	op := New(svcCfg, "svc-a", "", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error due to restart failure")
	}

	// Verify we restored to prior current (v1.1.0)
	current, _ := symlinkMgr.GetCurrent("/opt/a4-services/svc-a")
	if current != "v1.1.0" {
		t.Errorf("after failed restart, current = %q, want %q", current, "v1.1.0")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
