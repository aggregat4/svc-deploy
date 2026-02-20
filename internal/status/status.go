// Package status implements the status command.
package status

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/interfaces"
)

// Deps contains all external dependencies for status operations.
type Deps struct {
	FS         interfaces.FS
	ServiceMgr interfaces.ServiceManager
}

// Operation retrieves service status.
type Operation struct {
	cfg     config.ServiceConfig
	service string
	deps    Deps
}

// Result contains the status information.
type Result struct {
	CurrentVersion  string
	PreviousVersion string
	Active          bool
	Loaded          bool
	SubStatus       string
	Metadata        *ReleaseMetadata
}

// ReleaseMetadata represents the metadata stored in a release.
type ReleaseMetadata struct {
	Version      string `json:"version"`
	SHA256       string `json:"sha256"`
	DeployedAt   string `json:"deployed_at"`
	SourceURL    string `json:"source_url"`
	ConfigCommit string `json:"config_commit,omitempty"`
	DeployID     string `json:"deploy_id"`
}

// New creates a new status operation.
func New(cfg config.ServiceConfig, service string, deps Deps) *Operation {
	return &Operation{
		cfg:     cfg,
		service: service,
		deps:    deps,
	}
}

// Run retrieves the service status.
func (op *Operation) Run(ctx context.Context) (*Result, error) {
	servicePath := config.ServicePath(op.service)

	result := &Result{}

	// Resolve current release from symlink
	currentPath := config.CurrentPath(op.service)
	if target, err := op.deps.FS.Readlink(currentPath); err == nil {
		result.CurrentVersion = filepath.Base(target)

		// Load metadata if available
		metadataPath := filepath.Join(target, "metadata", "release.json")
		if data, err := op.deps.FS.ReadFile(metadataPath); err == nil {
			var meta ReleaseMetadata
			if err := json.Unmarshal(data, &meta); err == nil {
				result.Metadata = &meta
			}
		}
	}

	// Resolve previous release
	previousPath := config.PreviousPath(op.service)
	if target, err := op.deps.FS.Readlink(previousPath); err == nil {
		result.PreviousVersion = filepath.Base(target)
	}

	// Get systemd status
	svcStatus, err := op.deps.ServiceMgr.Status(ctx, op.cfg.SystemdUnit)
	if err == nil {
		result.Active = svcStatus.Active
		result.Loaded = svcStatus.Loaded
		result.SubStatus = svcStatus.SubStatus
	}

	// Check if service path exists
	if !op.deps.FS.Exists(servicePath) {
		return nil, fmt.Errorf("service %s not deployed (path %s does not exist)", op.service, servicePath)
	}

	return result, nil
}
