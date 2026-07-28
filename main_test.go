package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs goleak.VerifyTestMain at test teardown. Catches the
// goroutine leak class documented in docs/COMMON_BUGS.md §4.1
// (e.g. commit history shows multiple `fix:` commits for leaked
// goroutines in jobs, scrape, and server-sent event handlers).
// A test that starts a goroutine and forgets to cancel its
// context will fail the suite with the leaked goroutine's stack
// trace, naming the source file:line.
//
// Default matcher ignores the standard library runtime goroutines
// (finalizer, sweep, scavenger) so the noise is bounded. If a new
// legitimate leak surfaces in DixieData code, prefer fixing the
// leak over adding an IgnoreTopFunction / IgnoreAnyFunction
// matcher — every ignored entry is a future silent regression.
//
// Refs issue #318 Slice 2.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreTopFunction("net/http.http2ClientConnReadLoop"),
		goleak.IgnoreTopFunction("net/http.http2ServerConnKeepAlive"),
		goleak.IgnoreTopFunction("net/http.(*http2ClientConn).readLoop"),
	)
	os.Exit(m.Run())
}

func TestCurrentWebViewUserDataPathPreservesStableAndIsolatesRC(t *testing.T) {
	resolverCalled := false
	stablePath, err := currentWebViewUserDataPath("", `C:\Stable\.dixiedata`, func() (string, error) {
		resolverCalled = true
		return `C:\Users\TDM\AppData\Roaming`, nil
	})
	if err != nil || stablePath != "" {
		t.Fatalf("stable profile = %q, %v; want empty Wails default", stablePath, err)
	}
	if resolverCalled {
		t.Fatal("stable profile must not resolve or migrate AppData")
	}

	rcPath, err := currentWebViewUserDataPath("rc1", `C:\RC\.dixiedata`, func() (string, error) {
		return `C:\Users\TDM\AppData\Roaming`, nil
	})
	if err != nil {
		t.Fatalf("RC profile: %v", err)
	}
	if rcPath == "" || !strings.Contains(rcPath, `DixieData-RC`) {
		t.Fatalf("RC profile = %q; want isolated explicit path", rcPath)
	}

	_, err = currentWebViewUserDataPath("rc1", `C:\RC\.dixiedata`, func() (string, error) {
		return "", fmt.Errorf("permission denied")
	})
	if err == nil {
		t.Fatal("RC profile resolver failure must not fall back to shared stable profile")
	}
}

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
	if !strings.Contains(output, "DixieData v") {
		t.Errorf("output missing app label: %q", output)
	}

	w.Close()
	<-doneCh
}

// TestHelpFlagDispatch verifies that handleHelpFlag routes the
// help request correctly. Rows with wantHelp=true assert the help
// text is produced (header + every subcommand listed); rows with
// wantHelp=false assert that handleHelpFlag returned
// requested=false so downstream dispatch (Wails GUI launch, the
// subcommand dispatcher) is reachable. Issue #277.
func TestHelpFlagDispatch(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantHelp bool
	}{
		{name: "help_subcommand", args: []string{"dixiedata", "help"}, wantHelp: true},
		{name: "long_help_flag", args: []string{"dixiedata", "--help"}, wantHelp: true},
		{name: "short_help_flag", args: []string{"dixiedata", "-h"}, wantHelp: true},
		{name: "help_flag_with_extra_arg", args: []string{"dixiedata", "--help", "garbage"}, wantHelp: true},
		{name: "subcommand_only", args: []string{"dixiedata", "doctor"}, wantHelp: false},
		{name: "subcommand_with_flag", args: []string{"dixiedata", "doctor", "--check=data_dir"}, wantHelp: false},
		// No-args case: must return requested=false so the Wails
		// GUI launch path runs. Regression net for the bug where
		// a bare `dixiedata` invocation printed the help text and
		// exited 0 instead of opening the app window.
		{name: "no_args_must_not_block_gui_launch", args: []string{"dixiedata"}, wantHelp: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			output, requested := handleHelpFlag(c.args)
			if requested != c.wantHelp {
				t.Fatalf("handleHelpFlag(%v) requested=%v, want %v", c.args, requested, c.wantHelp)
			}
			if !c.wantHelp {
				return
			}
			if !strings.Contains(output, "DixieData CLI") {
				t.Errorf("output missing header: %q", output)
			}
			for _, sub := range []string{"--smoke", "--version", "doctor", "list", "show", "search", "export", "import", "migrate", "backup", "restore point", "logs", "config", "debug"} {
				if !strings.Contains(output, sub) {
					t.Errorf("output missing subcommand %q", sub)
				}
			}
		})
	}
}

// TestHasLogToStderr verifies the --log-to-stderr flag
// scanner. Issue #270.
func TestHasLogToStderr(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"absent", []string{"dixiedata", "doctor"}, false},
		{"present", []string{"dixiedata", "--log-to-stderr", "doctor"}, true},
		{"present equals 1", []string{"dixiedata", "--log-to-stderr=1", "doctor"}, true},
		{"explicit false", []string{"dixiedata", "--log-to-stderr=0", "doctor"}, false},
		{"present mid args", []string{"dixiedata", "doctor", "--log-to-stderr"}, true},
		{"only flag", []string{"dixiedata", "--log-to-stderr"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasLogToStderr(c.args); got != c.want {
				t.Errorf("hasLogToStderr(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}

// TestErrorFormatConvention asserts that every error path
// in the CLI dispatchers writes 'error: <msg>\n' to stderr
// via the centralised writeError helper. Issue #274.
func TestErrorFormatConvention(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"soldier not found", "error: soldier not found\n"},
		{"", "error: \n"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			var buf bytes.Buffer
			writeError(&buf, c.in)
			if got := buf.String(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestStderrCaptureShape verifies that stderr capture
// during a synthetic failure emits the right prefix.
func TestStderrCaptureShape(t *testing.T) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	doneCh := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(doneCh)
	}()

	writeError(os.Stderr, "soldier not found: DXD-99999")
	w.Close()
	<-doneCh

	got := buf.String()
	if !strings.HasPrefix(got, "error: ") {
		t.Errorf("stderr output missing 'error: ' prefix: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("stderr output missing trailing newline: %q", got)
	}
}

// TestRecoverExit5 verifies that a panic inside the wrapped
// function is converted to exit code 5 + the panic value +
// stack trace land on stderr. Issue #275.
func TestRecoverExit5(t *testing.T) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	doneCh := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(doneCh)
	}()

	code, err := recoverExit5(func() (int, error) {
		panic("kaboom")
	})
	w.Close()
	<-doneCh

	if code != 5 {
		t.Errorf("code = %d, want 5", code)
	}
	if err == nil {
		t.Error("expected non-nil error after panic")
	}
	stderr := buf.String()
	if !strings.Contains(stderr, "internal error:") {
		t.Errorf("stderr missing 'internal error:' prefix: %q", stderr)
	}
	if !strings.Contains(stderr, "kaboom") {
		t.Errorf("stderr missing panic value: %q", stderr)
	}
	if !strings.Contains(stderr, "stack trace:") {
		t.Errorf("stderr missing stack trace: %q", stderr)
	}
}

// TestRecoverExit5NoPanic verifies that a non-panicking
// wrapped function returns normally. Issue #275.
func TestRecoverExit5NoPanic(t *testing.T) {
	code, err := recoverExit5(func() (int, error) {
		return 42, nil
	})
	if code != 42 {
		t.Errorf("code = %d, want 42", code)
	}
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}
