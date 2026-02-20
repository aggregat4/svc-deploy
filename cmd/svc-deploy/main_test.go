package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseGlobalFlags tests the flag parsing logic.
func TestParseGlobalFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantFlags     cliFlags
		wantRemaining []string
		wantErr       bool
	}{
		{
			name:          "no flags",
			args:          []string{"deploy", "svc-a", "v1.0.0"},
			wantFlags:     cliFlags{},
			wantRemaining: []string{"deploy", "svc-a", "v1.0.0"},
		},
		{
			name:          "version flag only",
			args:          []string{"--version"},
			wantFlags:     cliFlags{showVer: true},
			wantRemaining: []string{},
		},
		{
			name:          "help flag only",
			args:          []string{"--help"},
			wantFlags:     cliFlags{showHelp: true},
			wantRemaining: []string{},
		},
		{
			name:          "json flag",
			args:          []string{"--json", "deploy", "svc-a", "v1.0.0"},
			wantFlags:     cliFlags{jsonOutput: true},
			wantRemaining: []string{"deploy", "svc-a", "v1.0.0"},
		},
		{
			name:          "config flag",
			args:          []string{"-c", "/path/to/config.toml", "status", "svc-a"},
			wantFlags:     cliFlags{configPath: "/path/to/config.toml"},
			wantRemaining: []string{"status", "svc-a"},
		},
		{
			name:          "flags after command",
			args:          []string{"deploy", "svc-a", "v1.0.0", "--json"},
			wantFlags:     cliFlags{jsonOutput: true},
			wantRemaining: []string{"deploy", "svc-a", "v1.0.0"},
		},
		{
			name:          "multiple flags mixed",
			args:          []string{"-c", "/config.toml", "--json", "status", "svc-a"},
			wantFlags:     cliFlags{configPath: "/config.toml", jsonOutput: true},
			wantRemaining: []string{"status", "svc-a"},
		},
		{
			name:    "unknown flag",
			args:    []string{"--unknown"},
			wantErr: true,
		},
		{
			name:    "config without value",
			args:    []string{"-c"},
			wantErr: true,
		},
		{
			name:          "double dash separator",
			args:          []string{"--json", "--", "deploy", "--config", "value"},
			wantFlags:     cliFlags{jsonOutput: true},
			wantRemaining: []string{"deploy", "--config", "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, remaining, err := parseGlobalFlags(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseGlobalFlags() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("parseGlobalFlags() unexpected error: %v", err)
				return
			}

			if flags.configPath != tt.wantFlags.configPath {
				t.Errorf("configPath = %q, want %q", flags.configPath, tt.wantFlags.configPath)
			}
			if flags.jsonOutput != tt.wantFlags.jsonOutput {
				t.Errorf("jsonOutput = %v, want %v", flags.jsonOutput, tt.wantFlags.jsonOutput)
			}
			if flags.showVer != tt.wantFlags.showVer {
				t.Errorf("showVer = %v, want %v", flags.showVer, tt.wantFlags.showVer)
			}
			if flags.showHelp != tt.wantFlags.showHelp {
				t.Errorf("showHelp = %v, want %v", flags.showHelp, tt.wantFlags.showHelp)
			}

			if len(remaining) != len(tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRemaining)
			} else {
				for i := range remaining {
					if remaining[i] != tt.wantRemaining[i] {
						t.Errorf("remaining[%d] = %q, want %q", i, remaining[i], tt.wantRemaining[i])
					}
				}
			}
		})
	}
}

// TestCLI_Version tests the --version flag works without config.
func TestCLI_Version(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Args = []string{"svc-deploy", "--version"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_Version")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Should exit 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 0 {
				t.Errorf("Expected exit code 0, got %d", exitErr.ExitCode())
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	output := stdout.String()
	if !strings.Contains(output, "svc-deploy") {
		t.Errorf("Expected output to contain 'svc-deploy', got: %s", output)
	}
	if !strings.Contains(output, "0.1.0") {
		t.Errorf("Expected output to contain version '0.1.0', got: %s", output)
	}
}

// TestCLI_Help tests the --help flag works without config.
func TestCLI_Help(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Args = []string{"svc-deploy", "--help"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_Help")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 0 {
				t.Errorf("Expected exit code 0, got %d", exitErr.ExitCode())
			}
		} else {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	output := stdout.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("Expected output to contain 'Usage:', got: %s", output)
	}
}

// TestCLI_NoCommand tests error when no command is provided.
func TestCLI_NoCommand(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Args = []string{"svc-deploy"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_NoCommand")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err == nil {
		t.Error("Expected error exit, got success")
		return
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Errorf("Expected ExitError, got: %v", err)
		return
	}

	if exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1, got %d", exitErr.ExitCode())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "no command specified") {
		t.Errorf("Expected 'no command specified' in stderr, got: %s", stderrStr)
	}
}

// TestCLI_UnknownCommand tests error on unknown command.
func TestCLI_UnknownCommand(t *testing.T) {
	// Create a temp config file so config loading succeeds
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "deploy-map.toml")
	content := `[service.test]
release_url_template = "https://example.com/{{.Version}}"
artifact_filename_template = "test-{{.Version}}.tar.gz"
binary_path = "bin/test"
healthcheck_url = "http://localhost:8080/healthz"
systemd_unit = "test.service"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	if os.Getenv("BE_CRASHER") == "1" {
		configFlag := os.Getenv("TEST_CONFIG")
		os.Args = []string{"svc-deploy", "-c", configFlag, "unknowncmd"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_UnknownCommand")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1", "TEST_CONFIG="+configPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err == nil {
		t.Error("Expected error exit, got success")
		return
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Errorf("Expected ExitError, got: %v", err)
		return
	}

	if exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1, got %d", exitErr.ExitCode())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "unknown command") {
		t.Errorf("Expected 'unknown command' in stderr, got: %s", stderrStr)
	}
}

// TestCLI_MissingConfig tests error when config is missing.
func TestCLI_MissingConfig(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Args = []string{"svc-deploy", "deploy", "svc-a", "v1.0.0"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_MissingConfig")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err == nil {
		t.Error("Expected error exit, got success")
		return
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Errorf("Expected ExitError, got: %v", err)
		return
	}

	if exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1, got %d", exitErr.ExitCode())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "loading config") {
		t.Errorf("Expected 'loading config' in stderr, got: %s", stderrStr)
	}
}

// TestCLI_DeployMissingArgs tests error when deploy args are missing.
func TestCLI_DeployMissingArgs(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "deploy-map.toml")
	content := `[service.test]
release_url_template = "https://example.com/{{.Version}}"
artifact_filename_template = "test-{{.Version}}.tar.gz"
binary_path = "bin/test"
healthcheck_url = "http://localhost:8080/healthz"
systemd_unit = "test.service"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	if os.Getenv("BE_CRASHER") == "1" {
		configFlag := os.Getenv("TEST_CONFIG")
		os.Args = []string{"svc-deploy", "-c", configFlag, "deploy"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_DeployMissingArgs")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1", "TEST_CONFIG="+configPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err == nil {
		t.Error("Expected error exit, got success")
		return
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Errorf("Expected ExitError, got: %v", err)
		return
	}

	if exitErr.ExitCode() != 1 {
		t.Errorf("Expected exit code 1, got %d", exitErr.ExitCode())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "usage:") {
		t.Errorf("Expected 'usage:' in stderr, got: %s", stderrStr)
	}
}

// TestCLI_JsonOutputShape tests JSON error output format.
func TestCLI_JsonOutputShape(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		os.Args = []string{"svc-deploy", "--json", "deploy", "svc-a", "v1.0.0"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_JsonOutputShape")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Should fail due to missing config, but output JSON error
	if err == nil {
		t.Error("Expected error exit")
		return
	}

	// JSON errors go to stdout in our implementation
	output := stdout.String()
	if !strings.Contains(output, `"success"`) || !strings.Contains(output, `"error"`) {
		t.Errorf("Expected JSON output with success and error fields, got: %s", output)
	}
}
