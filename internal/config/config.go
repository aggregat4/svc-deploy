// Package config handles parsing and validation of deploy-map.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// DefaultKeepReleases is the default number of releases to retain.
	DefaultKeepReleases = 5
	// DefaultStartupTimeout is the default timeout for service startup health check.
	DefaultStartupTimeout = 30
	// DefaultRollbackTimeout is the default timeout for rollback health check.
	DefaultRollbackTimeout = 30
	// DefaultMinDiskSpace is the default minimum free disk space required (1GB).
	DefaultMinDiskSpace = 1 << 30 // 1GB
)

// DeployMap is the root configuration structure.
type DeployMap struct {
	// Services maps service names to their configuration.
	Services map[string]ServiceConfig `toml:"service"`
}

// ServiceConfig defines configuration for a single service.
type ServiceConfig struct {
	// ReleaseURLTemplate is the URL template for fetching releases.
	// Supports {{.Version}} placeholder.
	ReleaseURLTemplate string `toml:"release_url_template"`
	// ArtifactFilenameTemplate is the filename template for the artifact.
	// Supports {{.Version}} and {{.Service}} placeholders.
	ArtifactFilenameTemplate string `toml:"artifact_filename_template"`
	// ChecksumFilenameTemplate is the filename template for the checksum file.
	// Supports {{.Version}} and {{.Service}} placeholders.
	ChecksumFilenameTemplate string `toml:"checksum_filename_template"`
	// BinaryPath is the relative path to the binary inside the artifact.
	BinaryPath string `toml:"binary_path"`
	// HealthCheckURL is the URL to poll for health checks.
	HealthCheckURL string `toml:"healthcheck_url"`
	// SystemdUnit is the name of the systemd service unit.
	SystemdUnit string `toml:"systemd_unit"`
	// DBFilename is the relative path to the SQLite database file.
	// Optional - if empty, no DB handling is performed.
	DBFilename string `toml:"db_filename"`
	// StartupTimeout is the timeout in seconds for health check during deploy.
	StartupTimeout int `toml:"startup_timeout"`
	// RollbackTimeout is the timeout in seconds for health check during rollback.
	RollbackTimeout int `toml:"rollback_timeout"`
	// KeepReleases is the number of releases to retain (default: 5).
	KeepReleases int `toml:"keep_releases"`
	// MinDiskSpace is the minimum required free disk space in bytes (default: 1GB).
	MinDiskSpace uint64 `toml:"min_disk_space"`
}

// Load reads and parses the deploy-map.toml from the given path.
func Load(path string) (*DeployMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg DeployMap
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Apply defaults and validate
	for name, svc := range cfg.Services {
		if svc.KeepReleases == 0 {
			svc.KeepReleases = DefaultKeepReleases
		}
		if svc.StartupTimeout == 0 {
			svc.StartupTimeout = DefaultStartupTimeout
		}
		if svc.RollbackTimeout == 0 {
			svc.RollbackTimeout = DefaultRollbackTimeout
		}
		if svc.MinDiskSpace == 0 {
			svc.MinDiskSpace = DefaultMinDiskSpace
		}
		cfg.Services[name] = svc
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadFromDefaultPaths attempts to load the config from canonical paths.
// Tries /opt/config-repo/a4-services/deploy-map.toml first,
// then falls back to /etc/a4-services/deploy-map.toml.
func LoadFromDefaultPaths() (*DeployMap, error) {
	paths := []string{
		"/opt/config-repo/a4-services/deploy-map.toml",
		"/etc/a4-services/deploy-map.toml",
	}

	var lastErr error
	for _, path := range paths {
		cfg, err := Load(path)
		if err == nil {
			return cfg, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("could not load config from any default path: %w", lastErr)
}

// Validate checks that all service configurations are valid.
func (dm *DeployMap) Validate() error {
	if len(dm.Services) == 0 {
		return fmt.Errorf("no services defined in deploy map")
	}

	for name, svc := range dm.Services {
		if err := svc.Validate(name); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that a service configuration has all required fields.
func (sc ServiceConfig) Validate(name string) error {
	if sc.ReleaseURLTemplate == "" {
		return fmt.Errorf("service %q: release_url_template is required", name)
	}
	if sc.ArtifactFilenameTemplate == "" {
		return fmt.Errorf("service %q: artifact_filename_template is required", name)
	}
	if sc.BinaryPath == "" {
		return fmt.Errorf("service %q: binary_path is required", name)
	}
	if sc.HealthCheckURL == "" {
		return fmt.Errorf("service %q: healthcheck_url is required", name)
	}
	if sc.SystemdUnit == "" {
		return fmt.Errorf("service %q: systemd_unit is required", name)
	}
	// Note: KeepReleases is validated after defaults are applied, so 0 here
	// just means "use default" and will be set to DefaultKeepReleases
	return nil
}

// GetService returns the configuration for a named service.
func (dm *DeployMap) GetService(name string) (ServiceConfig, bool) {
	svc, ok := dm.Services[name]
	return svc, ok
}

// ServicePath returns the base path for a service under /opt/a4-services.
func ServicePath(name string) string {
	return filepath.Join("/opt/a4-services", name)
}

// SecretsPath returns the path to the secrets file for a service.
func SecretsPath(name string) string {
	return filepath.Join("/etc/a4-services", name+".env")
}

// ConfigRepoPath returns the path to the runtime config in the config repo.
func ConfigRepoPath(name string) string {
	return filepath.Join("/opt/config-repo/a4-services/services", name, "runtime.env")
}

// ReleasePath returns the path to a specific release directory.
func ReleasePath(service, version string) string {
	return filepath.Join(ServicePath(service), "releases", version)
}

// CurrentPath returns the path to the current symlink.
func CurrentPath(service string) string {
	return filepath.Join(ServicePath(service), "current")
}

// PreviousPath returns the path to the previous symlink.
func PreviousPath(service string) string {
	return filepath.Join(ServicePath(service), "previous")
}

// SharedPath returns the path to the shared directory for a service.
func SharedPath(service string) string {
	return filepath.Join(ServicePath(service), "shared")
}

// BackupsPath returns the path to the backups directory for a service.
func BackupsPath(service string) string {
	return filepath.Join(SharedPath(service), "backups")
}

// SharedConfigPath returns the path to the shared config directory for a service.
func SharedConfigPath(service string) string {
	return filepath.Join(SharedPath(service), "config")
}

// SharedDataPath returns the path to the shared data directory for a service.
func SharedDataPath(service string) string {
	return filepath.Join(SharedPath(service), "data")
}

// RunPath returns the path to the run directory for a service.
func RunPath(service string) string {
	return filepath.Join(SharedPath(service), "run")
}

// LogsPath returns the path to the logs directory for a service.
func LogsPath(service string) string {
	return filepath.Join(ServicePath(service), "logs")
}

// HistoryPath returns the path to the deploy history log for a service.
func HistoryPath(service string) string {
	return filepath.Join(SharedPath(service), "deploy-history.log")
}

// LockPath returns the path to the lock file for a service.
func LockPath(service string) string {
	return filepath.Join("/var/lock", "svc-deploy-"+service+".lock")
}
