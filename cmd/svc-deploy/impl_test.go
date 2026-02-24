package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createTestTarGz creates a .tar.gz archive with specified entries for testing.
func createTestTarGz(entries []tarEntry) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzWriter)

	for _, e := range entries {
		hdr := &tar.Header{
			Name: e.name,
			Mode: e.mode,
			Size: int64(len(e.content)),
		}
		if e.isDir {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0755
		} else if e.isSymlink {
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = e.linkTarget
			hdr.Size = 0
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}

		if !e.isDir && !e.isSymlink && len(e.content) > 0 {
			if _, err := tw.Write(e.content); err != nil {
				return nil, err
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type tarEntry struct {
	name       string
	content    []byte
	mode       int64
	isDir      bool
	isSymlink  bool
	linkTarget string
}

func TestExtractTar_ValidArchive(t *testing.T) {
	fs := NewRealFS()
	tmpDir := t.TempDir()

	entries := []tarEntry{
		{name: "bin/", isDir: true, mode: 0755},
		{name: "bin/myapp", content: []byte("binary content"), mode: 0755},
		{name: "config/", isDir: true, mode: 0755},
		{name: "config/app.conf", content: []byte("setting=value"), mode: 0644},
	}

	data, err := createTestTarGz(entries)
	if err != nil {
		t.Fatalf("Failed to create test archive: %v", err)
	}

	dst := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	if err := fs.ExtractTar(bytes.NewReader(data), dst); err != nil {
		t.Fatalf("ExtractTar failed: %v", err)
	}

	// Verify files exist
	binPath := filepath.Join(dst, "bin", "myapp")
	if _, err := os.Stat(binPath); err != nil {
		t.Errorf("Expected binary file to exist: %v", err)
	}

	content, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}
	if string(content) != "binary content" {
		t.Errorf("Content mismatch: got %q, want %q", string(content), "binary content")
	}
}

func TestExtractTar_PathTraversal(t *testing.T) {
	fs := NewRealFS()
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		entryName   string
		expectError bool
	}{
		{
			name:        "simple_traversal",
			entryName:   "../evil.txt",
			expectError: true,
		},
		{
			name:        "nested_traversal",
			entryName:   "foo/../../evil.txt",
			expectError: true,
		},
		{
			name:        "absolute_path",
			entryName:   "/etc/passwd",
			expectError: true,
		},
		{
			name:        "valid_nested_path",
			entryName:   "foo/bar/baz.txt",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := []tarEntry{
				{name: tt.entryName, content: []byte("content"), mode: 0644},
			}

			data, err := createTestTarGz(entries)
			if err != nil {
				t.Fatalf("Failed to create test archive: %v", err)
			}

			dst := filepath.Join(tmpDir, tt.name)
			if err := os.MkdirAll(dst, 0755); err != nil {
				t.Fatalf("Failed to create dest dir: %v", err)
			}

			err = fs.ExtractTar(bytes.NewReader(data), dst)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for path %q, but got none", tt.entryName)
				} else if !strings.Contains(err.Error(), "traversal") && !strings.Contains(err.Error(), "absolute") {
					t.Errorf("Expected traversal/absolute error, got: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for path %q: %v", tt.entryName, err)
				}
			}
		})
	}
}

func TestExtractTar_SymlinkValidation(t *testing.T) {
	fs := NewRealFS()
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		linkPath   string
		linkTarget string
		expectErr  bool
	}{
		{
			name:       "safe_relative_symlink",
			linkPath:   "link",
			linkTarget: "target",
			expectErr:  false,
		},
		{
			name:       "symlink_traversal",
			linkPath:   "link",
			linkTarget: "../evil",
			expectErr:  true,
		},
		{
			name:       "absolute_symlink",
			linkPath:   "link",
			linkTarget: "/etc/passwd",
			expectErr:  true,
		},
		// F4 fix: Test nested symlink path that requires parent directory creation
		{
			name:       "nested_symlink_path",
			linkPath:   "lib/current/link",
			linkTarget: "../target",
			expectErr:  false,
		},
		{
			name:       "deeply_nested_symlink",
			linkPath:   "a/b/c/d/link",
			linkTarget: "../../target",
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := []tarEntry{
				{name: "target", content: []byte("target content"), mode: 0644},
				{name: tt.linkPath, isSymlink: true, linkTarget: tt.linkTarget},
			}

			data, err := createTestTarGz(entries)
			if err != nil {
				t.Fatalf("Failed to create test archive: %v", err)
			}

			dst := filepath.Join(tmpDir, tt.name)
			if err := os.MkdirAll(dst, 0755); err != nil {
				t.Fatalf("Failed to create dest dir: %v", err)
			}

			err = fs.ExtractTar(bytes.NewReader(data), dst)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error for symlink %q -> %q, but got none", tt.linkPath, tt.linkTarget)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestParseChecksumFile(t *testing.T) {
	// Valid SHA256 checksum for testing (64 hex chars)
	validChecksum := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	tests := []struct {
		name      string
		content   string
		want      string
		expectErr bool
	}{
		{
			name:    "simple_checksum",
			content: validChecksum,
			want:    validChecksum,
		},
		{
			name:    "gnu_format_with_filename",
			content: validChecksum + "  myfile.tar.gz",
			want:    validChecksum,
		},
		{
			name:    "uppercase_normalized",
			content: strings.ToUpper(validChecksum),
			want:    validChecksum,
		},
		{
			name:      "empty_content",
			content:   "",
			expectErr: true,
		},
		{
			name:      "whitespace_only",
			content:   "   \n\t  ",
			expectErr: true,
		},
		{
			name:      "invalid_non_hex",
			content:   strings.Repeat("g", 64), // 'g' is not hex
			expectErr: true,
		},
		{
			name:      "wrong_length_too_short",
			content:   "abc123",
			expectErr: true,
		},
		{
			name:      "wrong_length_too_long",
			content:   validChecksum + "abc",
			expectErr: true,
		},
		{
			name:    "multiline_file",
			content: validChecksum + "  file1.tar.gz\n" + validChecksum + "  file2.tar.gz",
			want:    validChecksum,
		},
		{
			name:      "checksum_with_special_chars",
			content:   strings.Repeat("@", 64), // '@' is not hex
			expectErr: true,
		},
		{
			name:      "checksum_63_chars",
			content:   validChecksum[:63], // One char short
			expectErr: true,
		},
		{
			name:      "checksum_65_chars",
			content:   validChecksum + "a", // One char too many
			expectErr: true,
		},
		{
			name:      "mixed_valid_invalid_hex",
			content:   strings.Repeat("abc123", 10) + "xyz", // 60 hex + 3 non-hex = 63 chars
			expectErr: true,
		},
		{
			name:    "leading_whitespace",
			content: "   \t\n" + validChecksum,
			want:    validChecksum,
		},
		{
			name:    "trailing_whitespace",
			content: validChecksum + "   \t\n",
			want:    validChecksum,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChecksumFile(tt.content)
			if tt.expectErr {
				if err == nil {
					t.Errorf("parseChecksumFile() expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("parseChecksumFile() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("parseChecksumFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseChecksumFile_SHA256Length(t *testing.T) {
	// Valid SHA256 checksum (64 hex characters)
	validChecksum := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	got, err := parseChecksumFile(validChecksum + "  filename.tar.gz")
	if err != nil {
		t.Errorf("Expected valid SHA256 to parse, got error: %v", err)
	}
	if got != validChecksum {
		t.Errorf("parseChecksumFile() = %q, want %q", got, validChecksum)
	}
}

// TestExtractTar_NotGzip verifies error on non-gzip input
func TestExtractTar_NotGzip(t *testing.T) {
	fs := NewRealFS()
	tmpDir := t.TempDir()

	// Create plain tar (not gzipped)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: "file.txt",
		Mode: 0644,
		Size: 5,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatalf("Failed to write content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Failed to close tar: %v", err)
	}

	dst := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}

	err := fs.ExtractTar(bytes.NewReader(buf.Bytes()), dst)
	if err == nil {
		t.Error("Expected error for non-gzip input, got none")
	} else if !strings.Contains(err.Error(), "decompressing gzip") {
		t.Errorf("Expected gzip error, got: %v", err)
	}
}
