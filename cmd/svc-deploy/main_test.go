package main

import (
	"bytes"
	"encoding/json"
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
			name:          "global json flag after command is parsed",
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
		// F1 fix tests - prune --keep should not be rejected by global parser
		{
			name:          "prune with keep flag before command",
			args:          []string{"--json", "prune", "svc-a", "--keep", "3"},
			wantFlags:     cliFlags{jsonOutput: true},
			wantRemaining: []string{"prune", "svc-a", "--keep", "3"},
		},
		{
			name:          "prune with keep and global json after command",
			args:          []string{"prune", "svc-a", "--keep", "3", "--json"},
			wantFlags:     cliFlags{jsonOutput: true},
			wantRemaining: []string{"prune", "svc-a", "--keep", "3"},
		},
		{
			name:          "prune with keep flag -c before",
			args:          []string{"-c", "/config.toml", "prune", "svc-a", "--keep", "3"},
			wantFlags:     cliFlags{configPath: "/config.toml"},
			wantRemaining: []string{"prune", "svc-a", "--keep", "3"},
		},
		{
			name:          "prune with unknown flag should pass through",
			args:          []string{"prune", "svc-a", "--unknown-flag"},
			wantFlags:     cliFlags{},
			wantRemaining: []string{"prune", "svc-a", "--unknown-flag"},
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
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got parse error: %v; output=%q", err, output)
	}
	if _, ok := parsed["success"]; !ok {
		t.Errorf("expected success field in JSON output: %s", output)
	}
	if _, ok := parsed["error"]; !ok {
		t.Errorf("expected error field in JSON output: %s", output)
	}
}

// TestCLI_JsonOutputShape_FlagAfterCommand verifies trailing global --json still works.
func TestCLI_JsonOutputShape_FlagAfterCommand(t *testing.T) {
	// Create a temp config file so we can trigger service-not-found JSON output.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "deploy-map.toml")
	content := `[service.svc-a]
release_url_template = "https://example.com/{{.Version}}"
artifact_filename_template = "svc-a-{{.Version}}.tar.gz"
binary_path = "bin/svc-a"
healthcheck_url = "http://localhost:8080/healthz"
systemd_unit = "svc-a.service"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	if os.Getenv("BE_CRASHER") == "1" {
		configFlag := os.Getenv("TEST_CONFIG")
		os.Args = []string{"svc-deploy", "-c", configFlag, "status", "missing-svc", "--json"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCLI_JsonOutputShape_FlagAfterCommand")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1", "TEST_CONFIG="+configPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}

	output := stdout.String()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("expected valid JSON output for trailing --json, got: %v; output=%q", err, output)
	}
	if success, ok := parsed["success"].(bool); !ok || success {
		t.Errorf("expected success=false in JSON output: %s", output)
	}
	if _, ok := parsed["error"]; !ok {
		t.Errorf("expected error field in JSON output: %s", output)
	}
}

func TestJSONStatusResponseIncludesActiveWhenFalse(t *testing.T) {
	payload := jsonStatusResponse{
		Success:         true,
		Service:         "svc-a",
		CurrentVersion:  "v1.0.0",
		PreviousVersion: "v0.9.0",
		Active:          false,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal status response: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal status response: %v", err)
	}

	if _, ok := parsed["active"]; !ok {
		t.Fatalf("expected active field in JSON: %s", string(data))
	}
	if active, ok := parsed["active"].(bool); !ok || active {
		t.Fatalf("expected active=false in JSON: %s", string(data))
	}
}

func TestJSONPruneResponseIncludesZeroCounts(t *testing.T) {
	payload := jsonPruneResponse{
		Success:   true,
		Removed:   0,
		Remaining: 0,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal prune response: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal prune response: %v", err)
	}

	if _, ok := parsed["removed"]; !ok {
		t.Fatalf("expected removed field in JSON: %s", string(data))
	}
	if _, ok := parsed["remaining"]; !ok {
		t.Fatalf("expected remaining field in JSON: %s", string(data))
	}
}
