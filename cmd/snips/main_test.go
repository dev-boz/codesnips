package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	t.Parallel()

	oldVersion := version
	version = "v0.1.0"
	t.Cleanup(func() {
		version = oldVersion
	})

	stdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = stdout
	})

	exitCode := run([]string{"--version"})

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	if exitCode != 0 {
		t.Fatalf("run(--version) exit code = %d, want 0", exitCode)
	}
	if got := strings.TrimSpace(buf.String()); got != "v0.1.0" {
		t.Fatalf("run(--version) output = %q, want %q", got, "v0.1.0")
	}
}

func TestRunWrapRejectsInvalidHeaderStyle(t *testing.T) {
	t.Parallel()

	stderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = writer
	t.Cleanup(func() {
		os.Stderr = stderr
	})

	exitCode := run([]string{"wrap", "--header-style", "nope"})

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if exitCode != 2 {
		t.Fatalf("run(wrap --header-style nope) exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(buf.String(), "invalid --header-style") {
		t.Fatalf("run(wrap --header-style nope) stderr = %q, want invalid header-style message", buf.String())
	}
}
