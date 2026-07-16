// cli_signal_test.go -- issue #597.
//
// RED tests for the SIGINT/SIGTERM → DB-close contract on the
// CLI runners. Each subcommand runner in main.go opens the SQLite
// DB via (*App).Startup, dispatches the verb, and relies on the
// deferred (*App).Shutdown to close the DB. Before this slice
// landed, the lifecycle ctx was context.Background() — uncancellable
// — and no Go signal handler was installed. SIGINT/SIGTERM
// terminated the process before any defer fired, leaving
// dixiedata.db-wal + dixiedata.db-shm sidecar files in the data
// directory.
//
// These tests build a small `dixiedata-test` binary via
// `go build -o`, spawn it against `dixiedata logs tail --follow`
// (the only existing verb that blocks on ctx.Done()), send
// SIGTERM once the DB is open, and assert:
//
//   - Process exits within 6 seconds (5s job-drain budget + 1s slack).
//   - Exit code is 0 (Shutdown completed normally).
//   - No dixiedata.db-wal or dixiedata.db-shm remains in the data dir.
//
// POSIX-only. Windows console-close maps to a best-effort signal
// that may not arrive before process termination; the Wails
// desktop binary is the recommended Windows entry point.
package appshell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// TestSignalTERM_DrainsDBOnBlockingVerb pins the contract for
// `dixiedata logs tail --follow`. The verb opens the DB then
// blocks on `select { case <-ctx.Done(): ... }`. SIGTERM must
// cancel the lifecycle ctx, return from the verb, run the
// deferred Shutdown, and close the DB — no WAL/SHM stragglers.
//
// RED (before Slice 1 lands): subprocess is killed by SIGTERM,
// deferred Shutdown never fires, WAL/SHM remain in data dir.
// GREEN (after Slice 1 lands): handler installed, ctx cancels,
// deferred Shutdown runs, WAL/SHM cleaned up.
func TestSignalTERM_DrainsDBOnBlockingVerb(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM signal-handler contract is POSIX-only (issue #597)")
	}

	dataDir := testtemp.New(t)

	// Build a hermetic test binary so the test does not depend
	// on a pre-built `bin/dixiedata` existing on the host.
	binDir := testtemp.New(t)
	binPath := filepath.Join(binDir.Path(), "dixiedata-test")
	buildCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binPath, ".")
	// Build from the repo root (the test runs inside
	// internal/appshell). `go build .` resolves the package from
	// the working directory, which the test framework sets to the
	// package dir — point it at the repo root explicitly.
	repoRoot := repoRootForTest(t)
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -o %s .: %v\n%s", binPath, err, out)
	}

	// Spawn the binary with the blocking verb. The
	// environment is scrubbed so DIXIEDATA_DATA_DIR from the
	// host does not leak into the subprocess.
	cmd := exec.Command(binPath, "logs", "tail", "--follow", "--data-dir", dataDir.Path())
	cmd.Env = append(os.Environ(), "DIXIEDATA_DATA_DIR="+dataDir.Path())
	setStartProcAttr(cmd)
	var stderrBuf, stdoutBuf trimmedBuffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start subprocess: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort reap if the test failed before the signal
		// was sent or before the subprocess exited.
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait()
	})

	// Wait for the DB to be open. The WAL file appears once
	// the first write hits the DB; `Startup` runs migrations
	// before returning, so the file should exist within ~2s on
	// a sane machine. Poll up to 5s for slack on slow CI.
	walPath := filepath.Join(dataDir.Path(), "dixiedata.db-wal")
	shmPath := filepath.Join(dataDir.Path(), "dixiedata.db-shm")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(walPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("WAL file did not appear within 5s — DB never opened?\nstderr: %s\nstdout: %s",
				stderrBuf.String(), stdoutBuf.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Inject SIGTERM. With the handler installed (Slice 1),
	// signal.NotifyContext cancels the lifecycle ctx; the
	// `logs tail` select returns; the deferred Shutdown runs.
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	// Wait for the process to exit. Budget = 5s job drain + 1s
	// slack = 6s total. SIGKILL is the escape hatch if the
	// handler misfires.
	exitDone := make(chan error, 1)
	go func() { exitDone <- cmd.Wait() }()
	select {
	case err := <-exitDone:
		if err != nil {
			// Accept exit code 0 (Shutdown completed normally).
			// Anything else means Shutdown did not run or the
			// signal handler misfired.
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.Fatalf("subprocess exit code %d (want 0): %v\nstderr: %s\nstdout: %s",
					exitErr.ExitCode(), err, stderrBuf.String(), stdoutBuf.String())
			}
			t.Fatalf("subprocess Wait: %v\nstderr: %s\nstdout: %s",
				err, stderrBuf.String(), stdoutBuf.String())
		}
	case <-time.After(6 * time.Second):
		_ = cmd.Process.Signal(syscall.SIGKILL)
		t.Fatalf("subprocess did not exit within 6s of SIGTERM — Shutdown never fired.\nstderr: %s\nstdout: %s",
			stderrBuf.String(), stdoutBuf.String())
	}

	// Post-condition: no WAL/SHM stragglers. Poll briefly to
	// tolerate OS cleanup lag.
	stragglers := []string{walPath, shmPath}
	for _, p := range stragglers {
		deadline := time.Now().Add(2 * time.Second)
		for {
			if _, err := os.Stat(p); os.IsNotExist(err) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("sidecar file %s remains after SIGTERM — DB did not close cleanly", p)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// TestSignalTERM_DrainsDBOnFastVerb pins the contract for a
// non-blocking verb (`dixiedata list soldiers`). The verb opens
// the DB, queries, and exits before the test can inject a
// signal — the post-condition still holds because the deferred
// Shutdown closes the DB before process exit. This test catches
// a regression where Shutdown is removed entirely from the
// fast-verb path (a different bug class than #597, but the same
// assertion covers both).
func TestSignalTERM_DrainsDBOnFastVerb(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM signal-handler contract is POSIX-only (issue #597)")
	}

	dataDir := testtemp.New(t)

	binDir := testtemp.New(t)
	binPath := filepath.Join(binDir.Path(), "dixiedata-test")
	buildCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binPath, ".")
	build.Dir = repoRootForTest(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -o %s .: %v\n%s", binPath, err, out)
	}

	cmd := exec.Command(binPath, "list", "soldiers", "--data-dir", dataDir.Path())
	cmd.Env = append(os.Environ(), "DIXIEDATA_DATA_DIR="+dataDir.Path())
	setStartProcAttr(cmd)
	var stderrBuf, stdoutBuf trimmedBuffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start subprocess: %v", err)
	}

	// Wait for the WAL to appear so we know the DB is open
	// before the verb finishes.
	walPath := filepath.Join(dataDir.Path(), "dixiedata.db-wal")
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(walPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			// Some fast verbs open + close the DB before the
			// poll resolves. Acceptable — skip the signal
			// injection and just assert the post-condition.
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("subprocess Wait: %v\nstderr: %s\nstdout: %s",
			err, stderrBuf.String(), stdoutBuf.String())
	}

	// Post-condition: no WAL/SHM stragglers.
	for _, p := range []string{walPath, filepath.Join(dataDir.Path(), "dixiedata.db-shm")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("sidecar file %s remains after fast-verb exit — DB did not close cleanly", p)
		}
	}
}

// repoRootForTest walks up from the test's working directory
// (internal/appshell) until it finds go.mod. Mirrors the helper
// in cli_mutate_test.go so the test stays self-contained.
func repoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %s", dir)
		}
		dir = parent
	}
}

// trimmedBuffer is a tiny io.Writer that caps retained bytes so
// a misbehaving subprocess cannot blow the test's memory budget.
// 8 KiB is enough to capture any plausible stderr error message.
type trimmedBuffer struct {
	buf []byte
	cap int
}

func (b *trimmedBuffer) Write(p []byte) (int, error) {
	if b.cap == 0 {
		b.cap = 8 * 1024
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.cap {
		// Keep the tail — that's where the actual error message
		// lands if the subprocess failed late.
		b.buf = b.buf[len(b.buf)-b.cap:]
	}
	return len(p), nil
}

func (b *trimmedBuffer) String() string { return string(b.buf) }