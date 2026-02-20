package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/a4/svc-deploy/internal/interfaces"
)

// RealFS implements interfaces.FS using the real filesystem.
type RealFS struct{}

func NewRealFS() *RealFS {
	return &RealFS{}
}

func (fs *RealFS) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (fs *RealFS) MkdirAll(path string, perm int) error {
	return os.MkdirAll(path, os.FileMode(perm))
}

func (fs *RealFS) Remove(path string) error {
	return os.Remove(path)
}

func (fs *RealFS) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *RealFS) CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

func (fs *RealFS) CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return fs.CopyFile(path, dstPath)
	})
}

func (fs *RealFS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (fs *RealFS) WriteFile(path string, data []byte, perm int) error {
	return os.WriteFile(path, data, os.FileMode(perm))
}

func (fs *RealFS) CreateCompressedBackup(src, dst string) error {
	file, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzWriter := gzip.NewWriter(file)
	defer func() { _ = gzWriter.Close() }()

	tarWriter := tar.NewWriter(gzWriter)
	defer func() { _ = tarWriter.Close() }()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = relPath

		if err := tarWriter.WriteHeader(hdr); err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			_, err = io.Copy(tarWriter, f)
			return err
		}

		return nil
	})
}

func (fs *RealFS) ExtractTar(r io.Reader, dst string) error {
	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dst, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}

	return nil
}

func (fs *RealFS) ListDirectory(path string) ([]interfaces.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	result := make([]interfaces.DirEntry, len(entries))
	for i, e := range entries {
		result[i] = interfaces.DirEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
		}
	}
	return result, nil
}

func (fs *RealFS) Stat(path string) (interfaces.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return interfaces.FileInfo{}, err
	}

	return interfaces.FileInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    int(info.Mode()),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}, nil
}

func (fs *RealFS) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (fs *RealFS) Readlink(path string) (string, error) {
	return os.Readlink(path)
}

func (fs *RealFS) DiskFree(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

// HTTPArtifactFetcher implements interfaces.ArtifactFetcher using HTTP.
type HTTPArtifactFetcher struct {
	client *http.Client
}

func NewHTTPArtifactFetcher() *HTTPArtifactFetcher {
	return &HTTPArtifactFetcher{
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (f *HTTPArtifactFetcher) Fetch(ctx context.Context, url string, checksumURL string) (io.ReadCloser, string, error) {
	// Fetch checksum
	checksumReq, err := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating checksum request: %w", err)
	}

	checksumResp, err := f.client.Do(checksumReq)
	if err != nil {
		return nil, "", fmt.Errorf("fetching checksum: %w", err)
	}
	defer func() { _ = checksumResp.Body.Close() }()

	if checksumResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("checksum fetch failed: %s", checksumResp.Status)
	}

	checksumData, err := io.ReadAll(checksumResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading checksum: %w", err)
	}

	expectedChecksum := strings.Fields(string(checksumData))[0]

	// Fetch artifact
	artifactReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating artifact request: %w", err)
	}

	artifactResp, err := f.client.Do(artifactReq)
	if err != nil {
		return nil, "", fmt.Errorf("fetching artifact: %w", err)
	}

	if artifactResp.StatusCode != http.StatusOK {
		_ = artifactResp.Body.Close()
		return nil, "", fmt.Errorf("artifact fetch failed: %s", artifactResp.Status)
	}

	// Read and verify checksum
	data, err := io.ReadAll(artifactResp.Body)
	_ = artifactResp.Body.Close()
	if err != nil {
		return nil, "", fmt.Errorf("reading artifact: %w", err)
	}

	hash := sha256.Sum256(data)
	actualChecksum := hex.EncodeToString(hash[:])

	if actualChecksum != expectedChecksum {
		return nil, "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return io.NopCloser(strings.NewReader(string(data))), actualChecksum, nil
}

// FileLocker implements interfaces.Locker using file-based locking.
type FileLocker struct{}

func NewFileLocker() *FileLocker {
	return &FileLocker{}
}

func (l *FileLocker) Acquire(service string) (func(), error) {
	lockPath := filepath.Join("/var/lock", "svc-deploy-"+service+".lock")

	// Ensure lock directory exists
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	// Try to acquire exclusive lock (non-blocking)
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("could not acquire lock for service %s: %w", service, err)
	}

	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

// SystemdManager implements interfaces.ServiceManager using systemctl.
type SystemdManager struct{}

func NewSystemdManager() *SystemdManager {
	return &SystemdManager{}
}

func (m *SystemdManager) Restart(ctx context.Context, unit string) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", unit)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl restart failed: %w: %s", err, string(output))
	}
	return nil
}

func (m *SystemdManager) Stop(ctx context.Context, unit string) error {
	cmd := exec.CommandContext(ctx, "systemctl", "stop", unit)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl stop failed: %w: %s", err, string(output))
	}
	return nil
}

func (m *SystemdManager) Status(ctx context.Context, unit string) (interfaces.ServiceStatus, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", unit, "--property=ActiveState,LoadState,SubState")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return interfaces.ServiceStatus{}, fmt.Errorf("systemctl show failed: %w: %s", err, string(output))
	}

	status := interfaces.ServiceStatus{Unit: unit}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ActiveState=") {
			status.Active = strings.TrimPrefix(line, "ActiveState=") == "active"
		} else if strings.HasPrefix(line, "LoadState=") {
			status.Loaded = strings.TrimPrefix(line, "LoadState=") == "loaded"
		} else if strings.HasPrefix(line, "SubState=") {
			status.SubStatus = strings.TrimPrefix(line, "SubState=")
		}
	}

	return status, nil
}

// HTTPHealthChecker implements interfaces.HealthChecker using HTTP.
type HTTPHealthChecker struct {
	client *http.Client
}

func NewHTTPHealthChecker() *HTTPHealthChecker {
	return &HTTPHealthChecker{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (h *HTTPHealthChecker) Check(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// AtomicSymlinkManager implements interfaces.SymlinkManager.
type AtomicSymlinkManager struct {
	fs interfaces.FS
}

func NewAtomicSymlinkManager(fs interfaces.FS) *AtomicSymlinkManager {
	return &AtomicSymlinkManager{fs: fs}
}

func (sm *AtomicSymlinkManager) GetCurrent(servicePath string) (string, error) {
	currentPath := filepath.Join(servicePath, "current")
	target, err := sm.fs.Readlink(currentPath)
	if err != nil {
		return "", err
	}
	return filepath.Base(target), nil
}

func (sm *AtomicSymlinkManager) GetPrevious(servicePath string) (string, error) {
	previousPath := filepath.Join(servicePath, "previous")
	target, err := sm.fs.Readlink(previousPath)
	if err != nil {
		return "", err
	}
	return filepath.Base(target), nil
}

func (sm *AtomicSymlinkManager) SetCurrent(servicePath, releaseVersion string) error {
	currentPath := filepath.Join(servicePath, "current")
	previousPath := filepath.Join(servicePath, "previous")
	newReleasePath := filepath.Join(servicePath, "releases", releaseVersion)
	tempLink := filepath.Join(servicePath, ".current.new")

	// Get current target for updating previous
	oldCurrent, err := sm.fs.Readlink(currentPath)
	if err == nil {
		// Update previous symlink
		_ = sm.fs.Remove(previousPath)
		_ = sm.fs.Symlink(oldCurrent, previousPath)
	}

	// Create new symlink atomically
	_ = sm.fs.Remove(tempLink)
	if err := sm.fs.Symlink(newReleasePath, tempLink); err != nil {
		return fmt.Errorf("creating temp symlink: %w", err)
	}

	if err := os.Rename(tempLink, currentPath); err != nil {
		_ = sm.fs.Remove(tempLink)
		return fmt.Errorf("renaming symlink: %w", err)
	}

	return nil
}

func (sm *AtomicSymlinkManager) RollbackCurrent(servicePath string) error {
	currentPath := filepath.Join(servicePath, "current")
	previousPath := filepath.Join(servicePath, "previous")

	previousTarget, err := sm.fs.Readlink(previousPath)
	if err != nil {
		return fmt.Errorf("reading previous symlink: %w", err)
	}

	// Get current before switching
	oldCurrent, _ := sm.fs.Readlink(currentPath)
	_ = oldCurrent

	// Atomically switch back
	tempLink := filepath.Join(servicePath, ".current.rollback")
	_ = sm.fs.Remove(tempLink)
	if err := sm.fs.Symlink(previousTarget, tempLink); err != nil {
		return fmt.Errorf("creating rollback symlink: %w", err)
	}

	if err := os.Rename(tempLink, currentPath); err != nil {
		_ = sm.fs.Remove(tempLink)
		return fmt.Errorf("renaming rollback symlink: %w", err)
	}

	// Update previous to point to what was current
	_ = sm.fs.Remove(previousPath)
	if oldCurrent != "" {
		_ = sm.fs.Symlink(oldCurrent, previousPath)
	}

	return nil
}

// GitConfigRepo implements interfaces.ConfigRepo.
type GitConfigRepo struct {
	path string
}

func NewGitConfigRepo(path string) *GitConfigRepo {
	return &GitConfigRepo{path: path}
}

func (r *GitConfigRepo) GetCurrentCommit() (string, error) {
	cmd := exec.Command("git", "-C", r.path, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting git commit: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (r *GitConfigRepo) GetRuntimeConfig(service string) ([]byte, error) {
	configPath := filepath.Join(r.path, "a4-services", "services", service, "runtime.env")
	return os.ReadFile(configPath)
}

// Mock implementations for testing

// MockArtifactFetcher is a test artifact fetcher using httptest.
type MockArtifactFetcher struct {
	server *httptest.Server
}

func NewMockArtifactFetcher() *MockArtifactFetcher {
	return &MockArtifactFetcher{}
}

func (m *MockArtifactFetcher) SetServer(server *httptest.Server) {
	m.server = server
}

func (m *MockArtifactFetcher) Fetch(ctx context.Context, url string, checksumURL string) (io.ReadCloser, string, error) {
	if m.server == nil {
		return nil, "", fmt.Errorf("no mock server configured")
	}
	return nil, "", nil
}
