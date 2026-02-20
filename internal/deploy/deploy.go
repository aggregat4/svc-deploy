// Package deploy implements the deployment flow.
package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/interfaces"
)

// Deps contains all external dependencies for deploy operations.
type Deps struct {
	FS            interfaces.FS
	Fetcher       interfaces.ArtifactFetcher
	Locker        interfaces.Locker
	ServiceMgr    interfaces.ServiceManager
	HealthChecker interfaces.HealthChecker
	SymlinkMgr    interfaces.SymlinkManager
	ConfigRepo    interfaces.ConfigRepo
	Clock         interfaces.Clock
}

// Operation performs a deployment.
type Operation struct {
	cfg     config.ServiceConfig
	service string
	version string
	deps    Deps
}

// Result contains the outcome of a deployment.
type Result struct {
	Version         string
	PreviousVersion string
	DeployedAt      time.Time
	ConfigCommit    string
	// Warnings contains non-fatal errors that occurred during deploy.
	// Deploy is considered successful even if warnings are present.
	Warnings []string
}

// Metadata represents the release metadata stored in metadata/release.json.
type Metadata struct {
	Version      string    `json:"version"`
	SHA256       string    `json:"sha256"`
	DeployedAt   time.Time `json:"deployed_at"`
	SourceURL    string    `json:"source_url"`
	ConfigCommit string    `json:"config_commit,omitempty"`
	DeployID     string    `json:"deploy_id"`
}

// New creates a new deployment operation.
func New(cfg config.ServiceConfig, service, version string, deps Deps) *Operation {
	return &Operation{
		cfg:     cfg,
		service: service,
		version: version,
		deps:    deps,
	}
}

// Run executes the deployment flow.
func (op *Operation) Run(ctx context.Context) (*Result, error) {
	// 1. Acquire lock
	release, err := op.deps.Locker.Acquire(op.service)
	if err != nil {
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	defer release()

	servicePath := config.ServicePath(op.service)
	releasePath := config.ReleasePath(op.service, op.version)

	// 2. Validate service exists in deploy map (already done by caller)

	// 3-4. Fetch and verify artifact
	artifactURL, err := op.buildArtifactURL()
	if err != nil {
		return nil, fmt.Errorf("building artifact URL: %w", err)
	}

	checksumURL, err := op.buildChecksumURL()
	if err != nil {
		return nil, fmt.Errorf("building checksum URL: %w", err)
	}

	artifactReader, checksum, err := op.deps.Fetcher.Fetch(ctx, artifactURL, checksumURL)
	if err != nil {
		return nil, fmt.Errorf("fetching artifact: %w", err)
	}
	defer func() { _ = artifactReader.Close() }()

	// 5. Create releases/<version>
	if err := op.deps.FS.MkdirAll(releasePath, 0755); err != nil {
		return nil, fmt.Errorf("creating release directory: %w", err)
	}

	// 6. Extract artifact
	if err := op.deps.FS.ExtractTar(artifactReader, releasePath); err != nil {
		_ = op.deps.FS.RemoveAll(releasePath)
		return nil, fmt.Errorf("extracting artifact: %w", err)
	}

	// Verify binary exists
	binaryPath := filepath.Join(releasePath, op.cfg.BinaryPath)
	if !op.deps.FS.Exists(binaryPath) {
		_ = op.deps.FS.RemoveAll(releasePath)
		return nil, fmt.Errorf("binary not found at expected path: %s", binaryPath)
	}

	// 7. Copy DB from current release (if applicable)
	if op.cfg.DBFilename != "" {
		if err := op.copyDatabase(servicePath, releasePath); err != nil {
			_ = op.deps.FS.RemoveAll(releasePath)
			return nil, fmt.Errorf("copying database: %w", err)
		}
	}

	// 8. Create pre-deploy backup (best effort - don't fail if backup doesn't work)
	if op.cfg.DBFilename != "" {
		_ = op.createBackup(servicePath)
	}

	// 9. Place/symlink runtime config to shared path
	configCommit, err := op.setupRuntimeConfig()
	if err != nil {
		_ = op.deps.FS.RemoveAll(releasePath)
		return nil, fmt.Errorf("setting up runtime config: %w", err)
	}

	// 10. Preflight checks before cutover
	if err := op.runPreflights(); err != nil {
		_ = op.deps.FS.RemoveAll(releasePath)
		return nil, fmt.Errorf("preflight failed: %w", err)
	}

	// Get previous version before switching
	previousVersion, _ := op.deps.SymlinkMgr.GetCurrent(servicePath)
	_ = previousVersion

	// 11. Update symlinks atomically
	if err := op.deps.SymlinkMgr.SetCurrent(servicePath, op.version); err != nil {
		_ = op.deps.FS.RemoveAll(releasePath)
		return nil, fmt.Errorf("switching current symlink: %w", err)
	}

	// 12. Restart service
	if err := op.deps.ServiceMgr.Restart(ctx, op.cfg.SystemdUnit); err != nil {
		// Try to rollback
		op.rollback(servicePath)
		return nil, fmt.Errorf("restarting service: %w", err)
	}

	// 13. Poll health endpoint
	healthCtx, cancel := context.WithTimeout(ctx, time.Duration(op.cfg.StartupTimeout)*time.Second)
	defer cancel()

	if err := op.waitForHealth(healthCtx); err != nil {
		// 14. Automatic rollback
		op.rollback(servicePath)
		return nil, fmt.Errorf("health check failed, rolled back: %w", err)
	}

	// 15. Write metadata and history
	// Errors here are captured as warnings - deploy is still considered successful
	// since the service is already running the new version.
	deployedAt := op.deps.Clock.Now()
	var warnings []string

	if err := op.writeMetadata(releasePath, checksum, configCommit, deployedAt, artifactURL); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to write metadata: %v", err))
	}
	if err := op.appendHistory(deployedAt, previousVersion, ""); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to write history: %v", err))
	}

	// 16. Prune old releases (best effort - don't fail deploy if prune fails)
	if err := op.pruneOldReleases(); err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to prune old releases: %v", err))
	}

	return &Result{
		Version:         op.version,
		PreviousVersion: previousVersion,
		DeployedAt:      deployedAt,
		ConfigCommit:    configCommit,
		Warnings:        warnings,
	}, nil
}

func (op *Operation) buildArtifactURL() (string, error) {
	// Build data for template substitution
	data := struct {
		Version  string
		Service  string
		Artifact string
	}{
		Version: op.version,
		Service: op.service,
	}

	// First render the artifact filename template
	artifactTmpl, err := template.New("artifact").Parse(op.cfg.ArtifactFilenameTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing artifact filename template: %w", err)
	}
	var artifactBuf bytes.Buffer
	if err := artifactTmpl.Execute(&artifactBuf, data); err != nil {
		return "", fmt.Errorf("executing artifact filename template: %w", err)
	}
	data.Artifact = artifactBuf.String()

	// Now render the full URL template
	tmpl, err := template.New("url").Parse(op.cfg.ReleaseURLTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing URL template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing URL template: %w", err)
	}

	return buf.String(), nil
}

func (op *Operation) buildChecksumURL() (string, error) {
	artifactURL, err := op.buildArtifactURL()
	if err != nil {
		return "", err
	}

	if op.cfg.ChecksumFilenameTemplate != "" {
		tmpl, err := template.New("checksum").Parse(op.cfg.ChecksumFilenameTemplate)
		if err != nil {
			return "", err
		}

		var buf bytes.Buffer
		data := struct {
			Version string
			Service string
		}{
			Version: op.version,
			Service: op.service,
		}

		if err := tmpl.Execute(&buf, data); err != nil {
			return "", err
		}

		// If it's an absolute URL, return it
		if strings.HasPrefix(buf.String(), "http") {
			return buf.String(), nil
		}

		// Otherwise, append to base URL
		baseURL := artifactURL[:strings.LastIndex(artifactURL, "/")]
		return baseURL + "/" + buf.String(), nil
	}

	// Default: append .sha256 to artifact URL
	return artifactURL + ".sha256", nil
}

func (op *Operation) copyDatabase(servicePath, releasePath string) error {
	currentVersion, err := op.deps.SymlinkMgr.GetCurrent(servicePath)
	if err != nil {
		// No current release, this is the first deploy - skip DB copy
		return nil
	}

	srcDB := filepath.Join(servicePath, "releases", currentVersion, "data", op.cfg.DBFilename)
	dstDir := filepath.Join(releasePath, "data")
	dstDB := filepath.Join(dstDir, op.cfg.DBFilename)

	if !op.deps.FS.Exists(srcDB) {
		// Source DB doesn't exist yet, create empty directory
		return op.deps.FS.MkdirAll(dstDir, 0755)
	}

	if err := op.deps.FS.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	// Copy main DB file
	if err := op.deps.FS.CopyFile(srcDB, dstDB); err != nil {
		return err
	}

	// Copy WAL file if exists
	walSrc := srcDB + "-wal"
	walDst := dstDB + "-wal"
	if op.deps.FS.Exists(walSrc) {
		if err := op.deps.FS.CopyFile(walSrc, walDst); err != nil {
			return err
		}
	}

	// Copy SHM file if exists
	shmSrc := srcDB + "-shm"
	shmDst := dstDB + "-shm"
	if op.deps.FS.Exists(shmSrc) {
		if err := op.deps.FS.CopyFile(shmSrc, shmDst); err != nil {
			return err
		}
	}

	return nil
}

func (op *Operation) createBackup(servicePath string) error {
	currentVersion, err := op.deps.SymlinkMgr.GetCurrent(servicePath)
	if err != nil {
		return err
	}

	backupDir := config.BackupsPath(op.service)
	if err := op.deps.FS.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	srcDB := filepath.Join(servicePath, "releases", currentVersion, "data", op.cfg.DBFilename)
	if !op.deps.FS.Exists(srcDB) {
		return nil
	}

	timestamp := op.deps.Clock.Now().UTC().Format("20060102-150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s-%s-pre-deploy.db.gz", op.service, timestamp))

	return op.deps.FS.CreateCompressedBackup(srcDB, backupFile)
}

// setupRuntimeConfig places the runtime config at the shared config path
// as required by the systemd contract: /opt/a4-services/<service>/shared/config/runtime.env
func (op *Operation) setupRuntimeConfig() (string, error) {
	// Ensure shared config directory exists
	sharedConfigDir := config.SharedConfigPath(op.service)
	if err := op.deps.FS.MkdirAll(sharedConfigDir, 0755); err != nil {
		return "", fmt.Errorf("creating shared config dir: %w", err)
	}

	// Try to get config from repo
	configData, err := op.deps.ConfigRepo.GetRuntimeConfig(op.service)
	if err != nil {
		// No runtime config is OK - remove any existing runtime.env
		runtimePath := filepath.Join(sharedConfigDir, "runtime.env")
		_ = op.deps.FS.Remove(runtimePath)
		commit, _ := op.deps.ConfigRepo.GetCurrentCommit()
		if commit == "" {
			commit = "unknown"
		}
		return commit, nil
	}

	// Write runtime config to shared path
	runtimePath := filepath.Join(sharedConfigDir, "runtime.env")
	if err := op.deps.FS.WriteFile(runtimePath, configData, 0644); err != nil {
		return "", fmt.Errorf("writing runtime config: %w", err)
	}

	// Get config commit
	commit, err := op.deps.ConfigRepo.GetCurrentCommit()
	if err != nil {
		commit = "unknown"
	}

	return commit, nil
}

func (op *Operation) waitForHealth(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timeout")
		case <-ticker.C:
			if err := op.deps.HealthChecker.Check(ctx, op.cfg.HealthCheckURL); err == nil {
				return nil
			}
		}
	}
}

func (op *Operation) rollback(servicePath string) {
	// Try to switch back to previous
	_ = op.deps.SymlinkMgr.RollbackCurrent(servicePath)
	_ = op.deps.ServiceMgr.Restart(context.Background(), op.cfg.SystemdUnit)
}

func (op *Operation) writeMetadata(releasePath, checksum, configCommit string, deployedAt time.Time, sourceURL string) error {
	metadataDir := filepath.Join(releasePath, "metadata")
	if err := op.deps.FS.MkdirAll(metadataDir, 0755); err != nil {
		return err
	}

	meta := Metadata{
		Version:      op.version,
		SHA256:       checksum,
		DeployedAt:   deployedAt,
		SourceURL:    sourceURL,
		ConfigCommit: configCommit,
		DeployID:     fmt.Sprintf("%s:%s:%d", op.service, op.version, deployedAt.Unix()),
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	metadataPath := filepath.Join(metadataDir, "release.json")
	return op.deps.FS.WriteFile(metadataPath, data, 0644)
}

func (op *Operation) appendHistory(deployedAt time.Time, previousVersion, reason string) error {
	historyPath := config.HistoryPath(op.service)
	historyDir := filepath.Dir(historyPath)

	if err := op.deps.FS.MkdirAll(historyDir, 0755); err != nil {
		return err
	}

	entry := fmt.Sprintf("[%s] DEPLOY %s (previous: %s)\n",
		deployedAt.UTC().Format(time.RFC3339),
		op.version,
		previousVersion)

	if reason != "" {
		entry = fmt.Sprintf("[%s] ROLLBACK to %s (reason: %s)\n",
			deployedAt.UTC().Format(time.RFC3339),
			op.version,
			reason)
	}

	// Append to history file
	existing, _ := op.deps.FS.ReadFile(historyPath)
	newContent := make([]byte, len(existing)+len(entry))
	copy(newContent, existing)
	copy(newContent[len(existing):], []byte(entry))

	return op.deps.FS.WriteFile(historyPath, newContent, 0644)
}

// pruneOldReleases removes old releases keeping only the configured number.
// This is a best-effort operation - errors are logged but not returned.
func (op *Operation) pruneOldReleases() error {
	servicePath := config.ServicePath(op.service)
	releasesPath := filepath.Join(servicePath, "releases")

	// Get current and previous versions (protected from pruning)
	protected := make(map[string]bool)

	currentPath := config.CurrentPath(op.service)
	if target, err := op.deps.FS.Readlink(currentPath); err == nil {
		protected[filepath.Base(target)] = true
	}

	previousPath := config.PreviousPath(op.service)
	if target, err := op.deps.FS.Readlink(previousPath); err == nil {
		protected[filepath.Base(target)] = true
	}

	// List all releases
	entries, err := op.deps.FS.ListDirectory(releasesPath)
	if err != nil {
		return err
	}

	// Collect release versions
	var releases []string
	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		if protected[entry.Name] {
			continue
		}
		releases = append(releases, entry.Name)
	}

	// Sort by version (newest first using simple string compare for now)
	// TODO: Use semantic versioning compare (R8)
	sort.Slice(releases, func(i, j int) bool {
		return releases[i] > releases[j]
	})

	toKeep := op.cfg.KeepReleases
	if toKeep < 1 {
		toKeep = config.DefaultKeepReleases
	}

	// Keep only the most recent N releases
	kept := 0
	for _, version := range releases {
		if kept < toKeep {
			kept++
			continue
		}

		// Remove this release
		releasePath := filepath.Join(releasesPath, version)
		_ = op.deps.FS.RemoveAll(releasePath)
	}

	return nil
}

// runPreflights performs preflight checks before cutover.
// Returns error if any preflight fails, aborting the deploy before symlink switch.
func (op *Operation) runPreflights() error {
	servicePath := config.ServicePath(op.service)

	// Check 1: Disk space
	freeSpace, err := op.deps.FS.DiskFree(servicePath)
	if err != nil {
		return fmt.Errorf("checking disk space: %w", err)
	}

	minSpace := op.cfg.MinDiskSpace
	if minSpace == 0 {
		minSpace = config.DefaultMinDiskSpace
	}

	if freeSpace < minSpace {
		return fmt.Errorf("insufficient disk space: %d bytes free, %d bytes required",
			freeSpace, minSpace)
	}

	// Check 2: Secrets file exists
	secretsPath := config.SecretsPath(op.service)
	if !op.deps.FS.Exists(secretsPath) {
		return fmt.Errorf("secrets file not found: %s", secretsPath)
	}

	// Check 3: Secrets file is readable (basic file stat)
	info, err := op.deps.FS.Stat(secretsPath)
	if err != nil {
		return fmt.Errorf("cannot stat secrets file: %w", err)
	}

	if info.Size == 0 {
		return fmt.Errorf("secrets file is empty: %s", secretsPath)
	}

	return nil
}
