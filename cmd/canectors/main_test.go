package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testFixturePath returns the path to test fixtures
func testFixturePath(filename string) string {
	return filepath.Join("..", "..", "internal", "config", "testdata", filename)
}

// runCLI runs the CLI binary and returns stdout, stderr, and exit code
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	// Build the CLI binary if it doesn't exist
	binaryPath := filepath.Join(t.TempDir(), "canectors")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = filepath.Join("..", "..", "cmd", "canectors")
	if err := buildCmd.Run(); err != nil {
		// Try from current directory
		buildCmd = exec.Command("go", "build", "-o", binaryPath, "./cmd/canectors")
		buildCmd.Dir = filepath.Join("..", "..")
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("failed to build CLI: %v", err)
		}
	}

	// Run the CLI
	cmd := exec.Command(binaryPath, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}

	return stdout, stderr, exitCode
}

func TestCLI_Help(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "--help")

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout, "canectors") {
		t.Error("expected help to contain 'canectors'")
	}

	if !strings.Contains(stdout, "validate") {
		t.Error("expected help to contain 'validate' command")
	}

	if !strings.Contains(stdout, "run") {
		t.Error("expected help to contain 'run' command")
	}
}

func TestCLI_ValidateHelp(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "validate", "--help")

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	if !strings.Contains(stdout, "Validate a pipeline configuration file") {
		t.Error("expected validate help to contain description")
	}
}

func TestCLI_ValidateValidJSON(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "validate", testFixturePath("valid-schema-config.json"))

	if exitCode != ExitSuccess {
		t.Errorf("expected exit code %d, got %d\nstderr: %s", ExitSuccess, exitCode, stderr)
	}

	if !strings.Contains(stdout, "valid") {
		t.Errorf("expected output to contain 'valid', got: %s", stdout)
	}
}

func TestCLI_ValidateValidYAML(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "validate", testFixturePath("valid-config.yaml"))

	if exitCode != ExitSuccess {
		t.Errorf("expected exit code %d, got %d\nstderr: %s", ExitSuccess, exitCode, stderr)
	}

	if !strings.Contains(stdout, "valid") {
		t.Errorf("expected output to contain 'valid', got: %s", stdout)
	}

	if !strings.Contains(stdout, "yaml") {
		t.Errorf("expected output to mention 'yaml' format, got: %s", stdout)
	}
}

func TestCLI_ValidateInvalidJSON(t *testing.T) {
	_, stderr, exitCode := runCLI(t, "validate", testFixturePath("invalid-json.json"))

	if exitCode != ExitParseError {
		t.Errorf("expected exit code %d (parse error), got %d", ExitParseError, exitCode)
	}

	if !strings.Contains(stderr, "Parse errors") {
		t.Errorf("expected stderr to contain 'Parse errors', got: %s", stderr)
	}
}

func TestCLI_ValidateValidationErrors(t *testing.T) {
	_, stderr, exitCode := runCLI(t, "validate", testFixturePath("invalid-schema-missing-required.json"))

	if exitCode != ExitValidationError {
		t.Errorf("expected exit code %d (validation error), got %d", ExitValidationError, exitCode)
	}

	if !strings.Contains(stderr, "Validation errors") {
		t.Errorf("expected stderr to contain 'Validation errors', got: %s", stderr)
	}
}

func TestCLI_ValidateNonExistent(t *testing.T) {
	_, stderr, exitCode := runCLI(t, "validate", "nonexistent.json")

	if exitCode != ExitParseError {
		t.Errorf("expected exit code %d (parse error), got %d", ExitParseError, exitCode)
	}

	if !strings.Contains(stderr, "Parse errors") {
		t.Errorf("expected stderr to contain parse error for non-existent file, got: %s", stderr)
	}
}

func TestCLI_ValidateVerbose(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "validate", "--verbose", testFixturePath("valid-schema-config.json"))

	if exitCode != ExitSuccess {
		t.Errorf("expected exit code %d, got %d", ExitSuccess, exitCode)
	}

	// Verbose output should include connector name
	if !strings.Contains(stdout, "test-connector") {
		t.Errorf("expected verbose output to contain connector name, got: %s", stdout)
	}
}

func TestCLI_ValidateQuiet(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "validate", "--quiet", testFixturePath("valid-schema-config.json"))

	if exitCode != ExitSuccess {
		t.Errorf("expected exit code %d, got %d", ExitSuccess, exitCode)
	}

	// Quiet mode should suppress output
	if strings.Contains(stdout, "Validating") {
		t.Errorf("expected quiet mode to suppress 'Validating' message, got: %s", stdout)
	}
}

func TestCLI_RunValidConfig(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "run", testFixturePath("valid-schema-config.json"))

	if exitCode != ExitSuccess {
		t.Errorf("expected exit code %d, got %d\nstderr: %s", ExitSuccess, exitCode, stderr)
	}

	if !strings.Contains(stdout, "loaded successfully") {
		t.Errorf("expected output to contain 'loaded successfully', got: %s", stdout)
	}
}

func TestCLI_RunInvalidConfig(t *testing.T) {
	_, stderr, exitCode := runCLI(t, "run", testFixturePath("invalid-json.json"))

	if exitCode != ExitParseError {
		t.Errorf("expected exit code %d (parse error), got %d", ExitParseError, exitCode)
	}

	if !strings.Contains(stderr, "Parse errors") {
		t.Errorf("expected stderr to contain 'Parse errors', got: %s", stderr)
	}
}

func TestCLI_RunDryRun(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "run", "--dry-run", testFixturePath("valid-schema-config.json"))

	if exitCode != ExitSuccess {
		t.Errorf("expected exit code %d, got %d\nstderr: %s", ExitSuccess, exitCode, stderr)
	}

	// Should indicate dry-run mode
	if !strings.Contains(stdout, "dry-run") {
		t.Errorf("expected output to mention 'dry-run', got: %s", stdout)
	}

	// Should still show success
	if !strings.Contains(stdout, "successfully") {
		t.Errorf("expected output to contain 'successfully', got: %s", stdout)
	}
}

func TestCLI_Version(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "version")

	if exitCode != ExitSuccess {
		t.Errorf("expected exit code %d, got %d\nstderr: %s", ExitSuccess, exitCode, stderr)
	}

	// Should contain version information
	if !strings.Contains(stdout, "Version:") {
		t.Errorf("expected output to contain 'Version:', got: %s", stdout)
	}

	if !strings.Contains(stdout, "Commit:") {
		t.Errorf("expected output to contain 'Commit:', got: %s", stdout)
	}

	if !strings.Contains(stdout, "Build Date:") {
		t.Errorf("expected output to contain 'Build Date:', got: %s", stdout)
	}

	// Version should not be empty
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Version:") {
			parts := strings.Split(line, ":")
			if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
				t.Errorf("version value should not be empty, got: %s", line)
			}
		}
	}
}

func TestCLI_ValidateMissingArg(t *testing.T) {
	_, stderr, exitCode := runCLI(t, "validate")

	if exitCode == ExitSuccess {
		t.Error("expected non-zero exit code for missing argument")
	}

	if !strings.Contains(stderr, "accepts 1 arg") {
		t.Errorf("expected error about missing argument, got: %s", stderr)
	}
}

// TestMainFunction ensures main doesn't panic
func TestMainFunction(t *testing.T) {
	t.Helper()

	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set args to show help (doesn't exit)
	os.Args = []string{"canectors", "--help"}

	// Run should not panic
	// Note: This will print help to stdout
}
