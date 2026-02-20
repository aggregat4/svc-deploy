// Package rollback implements the rollback flow.
package rollback

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/interfaces"
)

// Deps contains all external dependencies for rollback operations.
type Deps struct {
	FS            interfaces.FS
	Locker        interfaces.Locker
	ServiceMgr    interfaces.ServiceManager
	HealthChecker interfaces.HealthChecker
	SymlinkMgr    interfaces.SymlinkManager
	Clock         interfaces.Clock
}

// Operation performs a rollback.
type Operation struct {
	cfg           config.ServiceConfig
	service       string
	targetVersion string
	deps          Deps
}

// Result contains the outcome of a rollback.
type Result struct {
	Version         string
	PreviousVersion string
	RolledBackAt    time.Time
}

// New creates a new rollback operation.
func New(cfg config.ServiceConfig, service, targetVersion string, deps Deps) *Operation {
	return &Operation{
		cfg:           cfg,
		service:       service,
		targetVersion: targetVersion,
		deps:          deps,
	}
}

// Run executes the rollback flow.
func (op *Operation) Run(ctx context.Context) (*Result, error) {
	// 1. Acquire lock
	release, err := op.deps.Locker.Acquire(op.service)
	if err != nil {
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	defer release()

	servicePath := config.ServicePath(op.service)

	// 2. Resolve target release
	targetVersion := op.targetVersion
	if targetVersion == "" {
		// Use previous symlink
		prev, err := op.deps.SymlinkMgr.GetPrevious(servicePath)
		if err != nil {
			return nil, fmt.Errorf("no previous release to rollback to: %w", err)
		}
		targetVersion = prev
	}

	// 3. Validate release exists
	targetReleasePath := config.ReleasePath(op.service, targetVersion)
	if !op.deps.FS.Exists(targetReleasePath) {
		return nil, fmt.Errorf("target release %s does not exist", targetVersion)
	}

	// Verify binary exists
	binaryPath := filepath.Join(targetReleasePath, op.cfg.BinaryPath)
	if !op.deps.FS.Exists(binaryPath) {
		return nil, fmt.Errorf("binary not found in target release: %s", binaryPath)
	}

	// Verify DB if applicable
	if op.cfg.DBFilename != "" {
		dbPath := filepath.Join(targetReleasePath, "data", op.cfg.DBFilename)
		if !op.deps.FS.Exists(dbPath) {
			return nil, fmt.Errorf("database not found in target release: %s", dbPath)
		}
	}

	// Get current version before switching
	currentVersion, _ := op.deps.SymlinkMgr.GetCurrent(servicePath)
	_ = currentVersion

	// 4. Switch current to target
	if err := op.deps.SymlinkMgr.SetCurrent(servicePath, targetVersion); err != nil {
		return nil, fmt.Errorf("switching to target release: %w", err)
	}

	// 5. Restart service
	if err := op.deps.ServiceMgr.Restart(ctx, op.cfg.SystemdUnit); err != nil {
		// Try to restore prior current
		if currentVersion != "" {
			_, _ = op.deps.SymlinkMgr.SetCurrent(servicePath, currentVersion), op.deps.ServiceMgr.Restart(context.Background(), op.cfg.SystemdUnit)
		}
		return nil, fmt.Errorf("restarting service: %w", err)
	}

	// 6. Health check
	healthCtx, cancel := context.WithTimeout(ctx, time.Duration(op.cfg.RollbackTimeout)*time.Second)
	defer cancel()

	if err := op.waitForHealth(healthCtx); err != nil {
		// 7. Restore prior current
		if currentVersion != "" {
			_, _ = op.deps.SymlinkMgr.SetCurrent(servicePath, currentVersion), op.deps.ServiceMgr.Restart(context.Background(), op.cfg.SystemdUnit)
		}
		return nil, fmt.Errorf("health check failed, restored prior: %w", err)
	}

	// 8. Record rollback in history
	rolledBackAt := op.deps.Clock.Now()
	_ = op.appendHistory(rolledBackAt, targetVersion, currentVersion)

	return &Result{
		Version:         targetVersion,
		PreviousVersion: currentVersion,
		RolledBackAt:    rolledBackAt,
	}, nil
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

func (op *Operation) appendHistory(rolledBackAt time.Time, targetVersion, fromVersion string) error {
	historyPath := config.HistoryPath(op.service)
	historyDir := filepath.Dir(historyPath)

	if err := op.deps.FS.MkdirAll(historyDir, 0755); err != nil {
		return err
	}

	entry := fmt.Sprintf("[%s] ROLLBACK to %s (from: %s)\n",
		rolledBackAt.UTC().Format(time.RFC3339),
		targetVersion,
		fromVersion)

	// Append to history file
	existing, _ := op.deps.FS.ReadFile(historyPath)
	newContent := make([]byte, len(existing)+len(entry))
	copy(newContent, existing)
	copy(newContent[len(existing):], []byte(entry))

	return op.deps.FS.WriteFile(historyPath, newContent, 0644)
}
