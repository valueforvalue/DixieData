package appshell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/jobs"
)

// jobsLogFilename is the on-disk file the jobs Registry appends to.
//
// Lives under appdata.LogsDir(dataDir) (the sibling `.dixiedata-logs/`
// directory), NOT inside the data dir. The data dir is renamed
// atomically by replaceDataDir during a `.ddbak` import; an open file
// handle inside the data dir blocks that rename on Windows with
// 'Access is denied'. See docs/COMMON_BUGS.md §4.x and the existing
// appdata.LogsDir docstring for the same pattern shipped for
// app.log.jsonl in commit b9a30cc.
const jobsLogFilename = "jobs.jsonl"

// jobsLogPath returns the canonical on-disk path for the jobs log.
// Centralized so tests, the migration helper, and openJobsRegistry
// agree on the location.
func jobsLogPath(dataDir string) string {
	return filepath.Join(appdata.LogsDir(dataDir), jobsLogFilename)
}

// migrateLegacyJobsLog moves <dataDir>/jobs.jsonl to the new
// LogsDir-based path if the old file exists. Idempotent: returns nil
// when the legacy file is absent. Called once during startup, before
// openJobsRegistry runs, so the legacy file isn't lost and the
// Registry rehydrates from it transparently.
//
// On Windows, os.Rename is best-effort: it fails with 'Access is
// denied' if DixieData.exe or another process holds the file open.
// In that case we fall back to copy + remove; the copy preserves the
// rehydratable JSONL even when rename is denied. The legacy file is
// not deleted if the copy fails.
func migrateLegacyJobsLog(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	oldPath := filepath.Join(dataDir, jobsLogFilename)
	info, err := os.Stat(oldPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	_ = info // stat only; we don't care about size for migration
	newPath := jobsLogPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return fmt.Errorf("migrate jobs log: mkdir %s: %w", filepath.Dir(newPath), err)
	}
	if err := os.Rename(oldPath, newPath); err == nil {
		return nil
	} else {
		// Rename failed (likely Windows file lock on the legacy
		// file). Fall back to copy + remove.
		if copyErr := copyFileContents(oldPath, newPath); copyErr != nil {
			return fmt.Errorf("migrate jobs log: copy %s -> %s: %w (rename: %v)", oldPath, newPath, copyErr, err)
		}
		// Best-effort remove. If this fails (still locked), the
		// legacy file is harmless: openJobsRegistry only reads the
		// new path going forward; the rehydrated Registry will
		// re-create state from the copy.
		_ = os.Remove(oldPath)
	}
	return nil
}

// copyFileContents copies src to dst, creating or truncating dst.
// Returns nil on success. Used by migrateLegacyJobsLog as a fallback
// when os.Rename fails on Windows.
func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// openJobsRegistry constructs the in-process jobs.Registry, rehydrates
// it from the jobs log if that file exists, and attaches the same
// file as the append-only writer so subsequent state changes land on
// disk. Errors opening or replaying the log are non-fatal: the registry
// falls back to an empty in-memory state so a corrupt or missing log
// never blocks startup.
//
// The on-disk log grows by ~2 lines per job state change (queued ->
// running -> terminal). It is bounded by per-process job volume
// because we only append, never trim. A periodic compaction could be
// added later if the file grows too large; for the desktop app the
// typical archive export finishes in seconds and never produces more
// than a few dozen entries per session.
func openJobsRegistry(dataDir string) *jobs.Registry {
	if dataDir == "" {
		return jobs.NewWithConcurrency(jobsConcurrencyFromEnv())
	}
	// Ensure the data directory exists before opening the jobs log.
	// lifecycle.startup() calls openJobsRegistry(a.dataDir) before
	// db.Open() creates the directory, so the log file's parent may
	// not exist yet on a fresh install. Without MkdirAll, the log
	// open fails silently and every job state change is dropped
	// from the JSONL until the next app restart (which then
	// sees no log and starts empty). db.Open also MkdirAll's the
	// directory, but it runs AFTER us; we'd race the first
	// state-change broadcast.
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "jobs: mkdir %s failed: %v\n", dataDir, err)
	}
	logPath := jobsLogPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "jobs: mkdir %s failed: %v\n", filepath.Dir(logPath), err)
	}
	// Move any legacy .dixiedata/jobs.jsonl to the new location
	// before we try to read the new path. Idempotent: no-op if the
	// legacy file is absent (fresh install, or already migrated).
	// Migration failure is non-fatal: openJobsRegistry falls back
	// to an empty registry below; the next startup retries the move.
	if err := migrateLegacyJobsLog(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "jobs: migrate legacy log: %v\n", err)
	}
	reg, err := rehydrateJobsFromLog(logPath)
	if err != nil {
		// Non-fatal: log to stderr and start empty so the desktop app
		// still works on a partially-corrupted data dir.
		fmt.Fprintf(os.Stderr, "jobs: rehydrate %s failed: %v\n", logPath, err)
		reg = jobs.NewWithConcurrency(jobsConcurrencyFromEnv())
	}

	writer, err := openJobsLogWriter(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jobs: open %s for append failed: %v\n", logPath, err)
		return reg
	}
	reg.SetLogWriter(writer, writer)
	return reg
}

// rehydrateJobsFromLog reads the JSONL log at path (if any) into a new
// Registry. Returns an empty Registry when the file does not exist.
func rehydrateJobsFromLog(path string) (*jobs.Registry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return jobs.NewWithConcurrency(jobsConcurrencyFromEnv()), nil
		}
		return nil, err
	}
	defer f.Close()
	return jobs.NewFromLog(f)
}

// openJobsLogWriter opens path in append mode and returns the writer
// the Registry will use for state-change events.
func openJobsLogWriter(path string) (io.WriteCloser, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return f, nil
}