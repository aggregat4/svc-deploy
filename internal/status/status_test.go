package status

import (
	"context"
	"testing"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/interfaces"
	"github.com/a4/svc-deploy/internal/testutil"
)

func TestStatusFreshDeploy(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()
	svcMgr := testutil.NewMockServiceManager()

	// Setup service and release
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.0.0")

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcMgr.SetStatus("svc-a.service", interfaces.ServiceStatus{
		Active:    true,
		Loaded:    true,
		Unit:      "svc-a.service",
		SubStatus: "running",
	})

	svcCfg := config.ServiceConfig{
		SystemdUnit: "svc-a.service",
	}

	deps := Deps{
		FS:         fs,
		ServiceMgr: svcMgr,
	}

	op := New(svcCfg, "svc-a", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CurrentVersion != "v1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", result.CurrentVersion, "v1.0.0")
	}

	if result.PreviousVersion != "" {
		t.Errorf("PreviousVersion = %q, want empty", result.PreviousVersion)
	}

	if !result.Active {
		t.Error("Active = false, want true")
	}

	if !result.Loaded {
		t.Error("Loaded = false, want true")
	}

	if result.SubStatus != "running" {
		t.Errorf("SubStatus = %q, want %q", result.SubStatus, "running")
	}
}

func TestStatusWithPrevious(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()
	svcMgr := testutil.NewMockServiceManager()

	// Setup service and releases
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.1.0")
	fs.AddSymlink("/opt/a4-services/svc-a/previous", "/opt/a4-services/svc-a/releases/v1.0.0")

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.1.0")
	symlinkMgr.SetPreviousDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcMgr.SetStatus("svc-a.service", interfaces.ServiceStatus{
		Active:    true,
		Loaded:    true,
		Unit:      "svc-a.service",
		SubStatus: "running",
	})

	svcCfg := config.ServiceConfig{
		SystemdUnit: "svc-a.service",
	}

	deps := Deps{
		FS:         fs,
		ServiceMgr: svcMgr,
	}

	op := New(svcCfg, "svc-a", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CurrentVersion != "v1.1.0" {
		t.Errorf("CurrentVersion = %q, want %q", result.CurrentVersion, "v1.1.0")
	}

	if result.PreviousVersion != "v1.0.0" {
		t.Errorf("PreviousVersion = %q, want %q", result.PreviousVersion, "v1.0.0")
	}
}

func TestStatusInactive(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()
	svcMgr := testutil.NewMockServiceManager()

	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0")
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.0.0")

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcMgr.SetStatus("svc-a.service", interfaces.ServiceStatus{
		Active:    false,
		Loaded:    true,
		Unit:      "svc-a.service",
		SubStatus: "dead",
	})

	svcCfg := config.ServiceConfig{
		SystemdUnit: "svc-a.service",
	}

	deps := Deps{
		FS:         fs,
		ServiceMgr: svcMgr,
	}

	op := New(svcCfg, "svc-a", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Active {
		t.Error("Active = true, want false")
	}

	if result.SubStatus != "dead" {
		t.Errorf("SubStatus = %q, want %q", result.SubStatus, "dead")
	}
}

func TestStatusNotDeployed(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	svcMgr := testutil.NewMockServiceManager()

	// No service directory or symlinks

	svcCfg := config.ServiceConfig{
		SystemdUnit: "svc-a.service",
	}

	deps := Deps{
		FS:         fs,
		ServiceMgr: svcMgr,
	}

	op := New(svcCfg, "svc-a", deps)
	_, err := op.Run(ctx)

	if err == nil {
		t.Fatal("expected error for non-deployed service")
	}

	expectedMsg := "service svc-a not deployed"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("error = %q, expected to contain %q", err.Error(), expectedMsg)
	}
}

func TestStatusWithMetadata(t *testing.T) {
	ctx := context.Background()

	fs := testutil.NewMockFS()
	symlinkMgr := testutil.NewMockSymlinkManager()
	svcMgr := testutil.NewMockServiceManager()

	// Setup service and release with metadata
	fs.AddDir("/opt/a4-services/svc-a")
	fs.AddDir("/opt/a4-services/svc-a/releases/v1.0.0/metadata")
	metadata := `{
		"version": "v1.0.0",
		"sha256": "abc123",
		"deployed_at": "2024-01-01T00:00:00Z",
		"deploy_id": "svc-a:v1.0.0:1234567890"
	}`
	fs.AddFile("/opt/a4-services/svc-a/releases/v1.0.0/metadata/release.json", []byte(metadata))
	fs.AddSymlink("/opt/a4-services/svc-a/current", "/opt/a4-services/svc-a/releases/v1.0.0")

	symlinkMgr.SetCurrentDirect("/opt/a4-services/svc-a", "/opt/a4-services/svc-a/releases/v1.0.0")

	svcMgr.SetStatus("svc-a.service", interfaces.ServiceStatus{
		Active:    true,
		Loaded:    true,
		Unit:      "svc-a.service",
		SubStatus: "running",
	})

	svcCfg := config.ServiceConfig{
		SystemdUnit: "svc-a.service",
	}

	deps := Deps{
		FS:         fs,
		ServiceMgr: svcMgr,
	}

	op := New(svcCfg, "svc-a", deps)
	result, err := op.Run(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Metadata == nil {
		t.Fatal("expected metadata to be loaded")
	}

	if result.Metadata.Version != "v1.0.0" {
		t.Errorf("Metadata.Version = %q, want %q", result.Metadata.Version, "v1.0.0")
	}

	if result.Metadata.SHA256 != "abc123" {
		t.Errorf("Metadata.SHA256 = %q, want %q", result.Metadata.SHA256, "abc123")
	}

	if result.Metadata.DeployID != "svc-a:v1.0.0:1234567890" {
		t.Errorf("Metadata.DeployID = %q, want %q", result.Metadata.DeployID, "svc-a:v1.0.0:1234567890")
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
