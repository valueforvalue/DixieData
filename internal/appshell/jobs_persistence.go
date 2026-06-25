package appshell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/jobs"
)

// jobsLogFilename is the on-disk file the jobs Registry appends to.
// Kept inside the data dir so it follows the same backup / wipe rules
// as the rest of the Local Archive.
const jobsLogFilename = "jobs.jsonl"

// openJobsRegistry constructs the in-process jobs.Registry, rehydrates
// it from dataDir/jobs.jsonl if that file exists, and attaches the same
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
	logPath := filepath.Join(dataDir, jobsLogFilename)
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