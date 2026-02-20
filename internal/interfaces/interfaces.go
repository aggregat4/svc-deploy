// Package interfaces defines the core abstractions for svc-deploy.
// All side effects are abstracted behind these interfaces for testability.
package interfaces

import (
	"context"
	"io"
	"time"
)

// ArtifactFetcher downloads and verifies release artifacts.
type ArtifactFetcher interface {
	// Fetch downloads the artifact and its checksum, verifies the checksum,
	// and returns a reader for the artifact tarball.
	Fetch(ctx context.Context, url string, checksumURL string) (io.ReadCloser, string, error)
}

// FS abstracts filesystem operations.
type FS interface {
	// Exists returns true if the path exists.
	Exists(path string) bool
	// MkdirAll creates a directory and all parent directories.
	MkdirAll(path string, perm int) error
	// Remove removes a file or directory.
	Remove(path string) error
	// RemoveAll removes a directory and all its contents.
	RemoveAll(path string) error
	// CopyFile copies a file from src to dst.
	CopyFile(src, dst string) error
	// CopyDir copies a directory from src to dst recursively.
	CopyDir(src, dst string) error
	// ReadFile reads the contents of a file.
	ReadFile(path string) ([]byte, error)
	// WriteFile writes data to a file with the given permissions.
	WriteFile(path string, data []byte, perm int) error
	// CreateCompressedBackup creates a gzipped tar backup of the source path.
	CreateCompressedBackup(src, dst string) error
	// ExtractTar extracts a tarball to the destination directory.
	ExtractTar(r io.Reader, dst string) error
	// ListDirectory returns a list of entries in a directory.
	ListDirectory(path string) ([]DirEntry, error)
	// Stat returns file info for a path.
	Stat(path string) (FileInfo, error)
	// Symlink creates a symbolic link.
	Symlink(oldname, newname string) error
	// Readlink returns the destination of a symbolic link.
	Readlink(path string) (string, error)
	// DiskFree returns the available disk space in bytes for the given path.
	DiskFree(path string) (uint64, error)
}

// DirEntry represents a directory entry.
type DirEntry struct {
	Name  string
	IsDir bool
}

// FileInfo represents file metadata.
type FileInfo struct {
	Name    string
	Size    int64
	Mode    int
	ModTime time.Time
	IsDir   bool
}

// SymlinkManager handles atomic symlink switching.
type SymlinkManager interface {
	// GetCurrent returns the target of the current symlink.
	GetCurrent(servicePath string) (string, error)
	// GetPrevious returns the target of the previous symlink.
	GetPrevious(servicePath string) (string, error)
	// SetCurrent atomically updates the current symlink to point to the release.
	// The old current is moved to previous.
	SetCurrent(servicePath, releaseVersion string) error
	// RollbackCurrent switches current back to previous.
	RollbackCurrent(servicePath string) error
}

// ServiceManager abstracts systemd interactions.
type ServiceManager interface {
	// Restart restarts the service unit.
	Restart(ctx context.Context, unit string) error
	// Stop stops the service unit.
	Stop(ctx context.Context, unit string) error
	// Status returns the current status of the service unit.
	Status(ctx context.Context, unit string) (ServiceStatus, error)
}

// ServiceStatus represents the status of a systemd service.
type ServiceStatus struct {
	Active    bool
	Loaded    bool
	Unit      string
	SubStatus string
}

// HealthChecker polls service health endpoints.
type HealthChecker interface {
	// Check performs a health check against the given URL.
	// Returns nil if healthy, error otherwise.
	Check(ctx context.Context, url string) error
}

// Locker provides exclusive access control for service operations.
type Locker interface {
	// Acquire attempts to acquire a lock for the service.
	// Returns a release function and nil on success.
	Acquire(service string) (release func(), err error)
}

// Clock abstracts time for testability.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

// ConfigRepo abstracts the local git config repository.
type ConfigRepo interface {
	// GetCurrentCommit returns the current commit hash of the config repo.
	GetCurrentCommit() (string, error)
	// GetRuntimeConfig returns the runtime config content for a service.
	GetRuntimeConfig(service string) ([]byte, error)
}
