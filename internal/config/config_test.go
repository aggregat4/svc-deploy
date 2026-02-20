package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid config",
			content: `
[service.svc-a]
release_url_template = "https://github.com/org/svc-a/releases/download/{{.Version}}/{{.Artifact}}"
artifact_filename_template = "svc-a-{{.Version}}.tar.gz"
binary_path = "bin/svc-a"
healthcheck_url = "http://127.0.0.1:8080/healthz"
systemd_unit = "svc-a.service"
db_filename = "svc-a.db"

[service.svc-b]
release_url_template = "https://github.com/org/svc-b/releases/download/{{.Version}}/{{.Artifact}}"
artifact_filename_template = "svc-b-{{.Version}}.tar.gz"
binary_path = "bin/svc-b"
healthcheck_url = "http://127.0.0.1:8081/healthz"
systemd_unit = "svc-b.service"
keep_releases = 3
`,
			wantErr: false,
		},
		{
			name: "missing required fields",
			content: `
[service.svc-a]
release_url_template = "https://example.com"
`,
			wantErr: true,
		},
		{
			name: "empty config",
			content: `
# No services defined
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "deploy-map.toml")

			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			cfg, err := Load(configPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Load() unexpected error: %v", err)
				return
			}

			if len(cfg.Services) == 0 {
				t.Error("expected at least one service")
			}
		})
	}
}

func TestServiceConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ServiceConfig
		wantErr bool
	}{
		{
			name: "complete config",
			cfg: ServiceConfig{
				ReleaseURLTemplate:       "https://example.com/{{.Version}}",
				ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
				BinaryPath:               "bin/app",
				HealthCheckURL:           "http://localhost:8080/healthz",
				SystemdUnit:              "app.service",
			},
			wantErr: false,
		},
		{
			name: "missing release_url_template",
			cfg: ServiceConfig{
				ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
				BinaryPath:               "bin/app",
				HealthCheckURL:           "http://localhost:8080/healthz",
				SystemdUnit:              "app.service",
			},
			wantErr: true,
		},
		{
			name: "missing artifact_filename_template",
			cfg: ServiceConfig{
				ReleaseURLTemplate: "https://example.com/{{.Version}}",
				BinaryPath:         "bin/app",
				HealthCheckURL:     "http://localhost:8080/healthz",
				SystemdUnit:        "app.service",
			},
			wantErr: true,
		},
		{
			name: "missing binary_path",
			cfg: ServiceConfig{
				ReleaseURLTemplate:       "https://example.com/{{.Version}}",
				ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
				HealthCheckURL:           "http://localhost:8080/healthz",
				SystemdUnit:              "app.service",
			},
			wantErr: true,
		},
		{
			name: "missing healthcheck_url",
			cfg: ServiceConfig{
				ReleaseURLTemplate:       "https://example.com/{{.Version}}",
				ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
				BinaryPath:               "bin/app",
				SystemdUnit:              "app.service",
			},
			wantErr: true,
		},
		{
			name: "missing systemd_unit",
			cfg: ServiceConfig{
				ReleaseURLTemplate:       "https://example.com/{{.Version}}",
				ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
				BinaryPath:               "bin/app",
				HealthCheckURL:           "http://localhost:8080/healthz",
			},
			wantErr: true,
		},
		{
			name: "invalid keep_releases",
			cfg: ServiceConfig{
				ReleaseURLTemplate:       "https://example.com/{{.Version}}",
				ArtifactFilenameTemplate: "app-{{.Version}}.tar.gz",
				BinaryPath:               "bin/app",
				HealthCheckURL:           "http://localhost:8080/healthz",
				SystemdUnit:              "app.service",
				KeepReleases:             0,
			},
			wantErr: false, // Should apply default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate("test-service")
			if tt.wantErr && err == nil {
				t.Errorf("Validate() expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	content := `
[service.test]
release_url_template = "https://example.com/{{.Version}}"
artifact_filename_template = "app-{{.Version}}.tar.gz"
binary_path = "bin/app"
healthcheck_url = "http://localhost:8080/healthz"
systemd_unit = "app.service"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "deploy-map.toml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	svc, ok := cfg.GetService("test")
	if !ok {
		t.Fatal("expected service 'test' to exist")
	}

	if svc.KeepReleases != DefaultKeepReleases {
		t.Errorf("KeepReleases = %d, want %d", svc.KeepReleases, DefaultKeepReleases)
	}

	if svc.StartupTimeout != DefaultStartupTimeout {
		t.Errorf("StartupTimeout = %d, want %d", svc.StartupTimeout, DefaultStartupTimeout)
	}

	if svc.RollbackTimeout != DefaultRollbackTimeout {
		t.Errorf("RollbackTimeout = %d, want %d", svc.RollbackTimeout, DefaultRollbackTimeout)
	}
}

func TestPathHelpers(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(string) string
		input    string
		expected string
	}{
		{"ServicePath", ServicePath, "svc-a", "/opt/a4-services/svc-a"},
		{"SecretsPath", SecretsPath, "svc-a", "/etc/a4-services/svc-a.env"},
		{"SharedPath", SharedPath, "svc-a", "/opt/a4-services/svc-a/shared"},
		{"BackupsPath", BackupsPath, "svc-a", "/opt/a4-services/svc-a/shared/backups"},
		{"CurrentPath", CurrentPath, "svc-a", "/opt/a4-services/svc-a/current"},
		{"PreviousPath", PreviousPath, "svc-a", "/opt/a4-services/svc-a/previous"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			if got != tt.expected {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.input, got, tt.expected)
			}
		})
	}
}

func TestReleasePath(t *testing.T) {
	got := ReleasePath("svc-a", "v1.0.0")
	expected := "/opt/a4-services/svc-a/releases/v1.0.0"
	if got != expected {
		t.Errorf("ReleasePath(%q, %q) = %q, want %q", "svc-a", "v1.0.0", got, expected)
	}
}
