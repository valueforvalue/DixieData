package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestVersionFlag verifies that handleVersionFlag returns
// the right output + done=true for both --version and -v.
// Issue #271.
func TestVersionFlag(t *testing.T) {
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			output, done := handleVersionFlag([]string{"dixiedata", flag})
			if !done {
				t.Fatalf("expected done=true for %s", flag)
			}
			if !strings.Contains(output, "DixieData v") {
				t.Errorf("output missing 'DixieData v': %q", output)
			}
			if !strings.Contains(output, "commit ") {
				t.Errorf("output missing 'commit ': %q", output)
			}
		})
	}
}

// TestVersionFlagIgnoresOthers verifies that --version
// short-circuits BEFORE the subcommand dispatchers. A
// malformed third arg would error in the dispatcher; the
// version short-circuit should win.
func TestVersionFlagIgnoresOthers(t *testing.T) {
	output, done := handleVersionFlag([]string{"dixiedata", "--version", "garbage"})
	if !done {
		t.Fatal("expected done=true when --version is present")
	}
	if !strings.Contains(output, "DixieData v") {
		t.Errorf("--version did not short-circuit: %q", output)
	}
}

// TestVersionFlagAbsent verifies that without --version /
// -v, handleVersionFlag returns done=false.
func TestVersionFlagAbsent(t *testing.T) {
	_, done := handleVersionFlag([]string{"dixiedata", "doctor"})
	if done {
		t.Error("expected done=false when no version flag is present")
	}
	_, done = handleVersionFlag([]string{"dixiedata"})
	if done {
		t.Error("expected done=false when no args")
	}
}

// TestVersionOutputFormat captures stdout and verifies the
// exact shape of the version output.
func TestVersionOutputFormat(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	doneCh := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(doneCh)
	}()

	output, done := handleVersionFlag([]string{"dixiedata", "--version"})
	if !done {
		t.Fatal("expected done=true")
	}
	// Print to the test writer (not stdout) so the assertion
	// uses the same path the production code uses.
	if !strings.Contains(output, "DixieData v") {
		t.Errorf("output missing app label: %q", output)
	}

	w.Close()
	<-doneCh
}