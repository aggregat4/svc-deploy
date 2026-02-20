// Package integrationtest provides utilities for integration testing with real artifacts.
package integrationtest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Artifact represents a test artifact with its content and checksum.
type Artifact struct {
	Content  []byte
	Checksum string
}

// CreateArtifact creates a real .tar.gz artifact for a service version.
// It creates a tarball with the expected structure:
//   - bin/<service> (binary)
//   - data/ (directory for database)
func CreateArtifact(service, version string, binaryContent []byte) (*Artifact, error) {
	var buf bytes.Buffer

	// Create gzip writer
	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()

	// Create tar writer
	tw := tar.NewWriter(gzWriter)
	defer tw.Close()

	// Add bin/ directory
	binDirHdr := &tar.Header{
		Name:     "bin/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(binDirHdr); err != nil {
		return nil, fmt.Errorf("writing bin dir header: %w", err)
	}

	// Add binary file
	binaryHdr := &tar.Header{
		Name: filepath.Join("bin", service),
		Mode: 0755,
		Size: int64(len(binaryContent)),
	}
	if err := tw.WriteHeader(binaryHdr); err != nil {
		return nil, fmt.Errorf("writing binary header: %w", err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		return nil, fmt.Errorf("writing binary content: %w", err)
	}

	// Add data/ directory
	dataDirHdr := &tar.Header{
		Name:     "data/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tw.WriteHeader(dataDirHdr); err != nil {
		return nil, fmt.Errorf("writing data dir header: %w", err)
	}

	// Close tar writer to flush
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar writer: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip writer: %w", err)
	}

	content := buf.Bytes()

	// Calculate SHA256 checksum
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	return &Artifact{
		Content:  content,
		Checksum: checksum,
	}, nil
}

// FixtureSet represents a collection of test artifacts for a service.
type FixtureSet struct {
	Service   string
	Artifacts map[string]*Artifact // version -> artifact
	TempDir   string
}

// SetupTestFixtures creates a set of test artifacts for integration testing.
// Returns a FixtureSet that should be cleaned up with Cleanup() when done.
func SetupTestFixtures(service string, versions []string) (*FixtureSet, error) {
	tempDir, err := os.MkdirTemp("", "svc-deploy-test-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	fs := &FixtureSet{
		Service:   service,
		Artifacts: make(map[string]*Artifact),
		TempDir:   tempDir,
	}

	for _, version := range versions {
		// Create artifact with version-specific content
		binaryContent := []byte(fmt.Sprintf("#!/bin/bash\necho 'Service: %s, Version: %s'\n", service, version))

		artifact, err := CreateArtifact(service, version, binaryContent)
		if err != nil {
			fs.Cleanup()
			return nil, fmt.Errorf("creating artifact for %s: %w", version, err)
		}

		fs.Artifacts[version] = artifact
	}

	return fs, nil
}

// Cleanup removes the temporary directory and all artifacts.
func (fs *FixtureSet) Cleanup() {
	if fs.TempDir != "" {
		_ = os.RemoveAll(fs.TempDir)
	}
}

// WriteArtifactToFile writes an artifact to disk for serving via HTTP.
func (fs *FixtureSet) WriteArtifactToFile(version, path string) error {
	artifact, ok := fs.Artifacts[version]
	if !ok {
		return fmt.Errorf("artifact not found for version %s", version)
	}

	if err := os.WriteFile(path, artifact.Content, 0644); err != nil {
		return fmt.Errorf("writing artifact file: %w", err)
	}

	return nil
}

// WriteChecksumToFile writes a checksum file for an artifact.
func (fs *FixtureSet) WriteChecksumToFile(version, path string) error {
	artifact, ok := fs.Artifacts[version]
	if !ok {
		return fmt.Errorf("artifact not found for version %s", version)
	}

	// Write checksum in GNU format: "<checksum>  <filename>"
	content := fmt.Sprintf("%s  %s-%s.tar.gz\n", artifact.Checksum, fs.Service, version)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing checksum file: %w", err)
	}

	return nil
}

// CreateTarGzFromFiles creates a .tar.gz from a map of file paths to content.
// This is useful for creating custom artifacts with specific file structures.
func CreateTarGzFromFiles(files map[string][]byte) (*Artifact, error) {
	var buf bytes.Buffer

	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()

	tw := tar.NewWriter(gzWriter)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}

		// Handle directories
		if len(content) == 0 && (name[len(name)-1] == '/') {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0755
			hdr.Size = 0
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("writing header for %s: %w", name, err)
		}

		if len(content) > 0 {
			if _, err := tw.Write(content); err != nil {
				return nil, fmt.Errorf("writing content for %s: %w", name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing tar writer: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip writer: %w", err)
	}

	content := buf.Bytes()
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	return &Artifact{
		Content:  content,
		Checksum: checksum,
	}, nil
}

// CopyReader copies data from a reader and returns it as a byte slice.
// This is useful for reading from HTTP response bodies.
func CopyReader(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
