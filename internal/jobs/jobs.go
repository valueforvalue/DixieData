// Package jobs provides a small in-process background-job registry used by
// the export handlers that would otherwise block the HTTP request goroutine
// for minutes on large archives. Jobs live in memory only; nothing here is
// persisted across app restarts (that is a separate concern tracked under
// audit issue #100 out-of-scope list).
//
// Usage:
//
//	reg := jobs.New()
//	id := reg.Start("static_archive", func(ctx context.Context, p *jobs.Progress) error {
//	    p.Set(0, "starting")
//	    return svc.ExportStaticArchive(path, dataDir, p)
//	})
//
// Each job exposes a status string ("queued", "running", "done",
// "error", "cancelled"), an integer progress 0-100, a started and finished
// timestamp, an error message, and an optional ResultPath populated by the
// worker before it marks the job done.
//
// Cancellation is cooperative: the worker must honour ctx.Done or call
// progress.Cancelled() periodically and return early.
package jobs

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Status values.
const (
	StatusQueued       = "queued"
	StatusRunning      = "running"
	StatusDone         = "done"
	StatusError        = "error"
	StatusCancelled    = "cancelled"
	StatusInterrupted  = "interrupted"
)

// Job is the registry-side view of a background job. Worker code should
// not write to this struct directly; it should use the Progress receiver
// passed to the worker function.
type Job struct {
	ID          string
	Kind        string
	Status      string
	Progress    int
	Message     string
	StartedAt   time.Time
	FinishedAt  time.Time
	Error       string
	ResultPath  string
	mu          sync.Mutex
	cancelled   bool
	cancelCause context.CancelFunc
}

// Progress is passed to a worker so it can update its job without holding
// the registry lock for the entire export.
type Progress struct {
	job *Job
}

// Set updates progress (0-100) and an optional human-readable message.
func (p *Progress) Set(percent int, message string) {
	if p == nil || p.job == nil {
		return
	}
	p.job.mu.Lock()
	defer p.job.mu.Unlock()
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	p.job.Progress = percent
	if message != "" {
		p.job.Message = message
	}
}

// Cancelled reports whether the job was cancelled. Workers should check
// this periodically and return early when it returns true.
func (p *Progress) Cancelled() bool {
	if p == nil || p.job == nil {
		return false
	}
	p.job.mu.Lock()
	defer p.job.mu.Unlock()
	return p.job.cancelled
}

// Snapshot returns the registry view of the job's current state.
func (j *Job) Snapshot() Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Job{
		ID:         j.ID,
		Kind:       j.Kind,
		Status:     j.Status,
		Progress:   j.Progress,
		Message:    j.Message,
		StartedAt:  j.StartedAt,
		FinishedAt: j.FinishedAt,
		Error:      j.Error,
		ResultPath: j.ResultPath,
	}
}

// DefaultConcurrency caps the number of jobs running in parallel when a
// caller uses New(). Two is enough to keep the desktop app responsive
// while letting the user kick off a backup export alongside a printable
// PDF without burning memory on a giant worker fan-out.
const DefaultConcurrency = 2

// Registry holds the live jobs for a process.
type Registry struct {
	mu          sync.Mutex
	jobs        map[string]*Job
	concurrency int
	sem         chan struct{}

	// logMu guards logWriter + logCloser. logWriter is appended to
	// on every job state change so the Registry survives a webview
	// reload or app restart. nil disables persistence.
	logMu     sync.Mutex
	logWriter io.Writer
	logCloser io.Closer
}

// persistedSnapshot is the on-disk shape of a job record. Stable across
// releases; do not rename fields without a migration.
type persistedSnapshot struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress"`
	Message     string    `json:"message,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	Error       string    `json:"error,omitempty"`
	ResultPath  string    `json:"result_path,omitempty"`
}

// New returns a Registry sized to DefaultConcurrency workers. Callers
// that need a different pool size should use NewWithConcurrency.
func New() *Registry {
	return NewWithConcurrency(DefaultConcurrency)
}

// NewWithConcurrency returns a Registry that allows at most n jobs to
// run in parallel. n <= 0 falls back to DefaultConcurrency.
func NewWithConcurrency(n int) *Registry {
	if n < 1 {
		n = DefaultConcurrency
	}
	return &Registry{
		jobs:        map[string]*Job{},
		concurrency: n,
		sem:         make(chan struct{}, n),
	}
}

// NewFromLog rehydrates a Registry from a JSONL stream previously
// produced by SetLogWriter. Jobs that were StatusRunning when the
// previous process exited are flipped to StatusInterrupted so the UI
// can show an honest 'lost when the app restarted' state instead of
// pretending the worker is still alive.
//
// The returned Registry is in-memory only and will not write back to
// the reader; call SetLogWriter after NewFromLog if you want the
// rehydrated entries to be re-appended to a new log.
func NewFromLog(reader io.Reader) (*Registry, error) {
	reg := NewWithConcurrency(DefaultConcurrency)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var snap persistedSnapshot
		if err := json.Unmarshal([]byte(line), &snap); err != nil {
			return nil, fmt.Errorf("jobs: parse JSONL line %d: %w", lineNo, err)
		}
		status := snap.Status
		if status == StatusQueued || status == StatusRunning {
			status = StatusInterrupted
		}
		job := &Job{
			ID:         snap.ID,
			Kind:       snap.Kind,
			Status:     status,
			Progress:   snap.Progress,
			Message:    snap.Message,
			StartedAt:  snap.StartedAt,
			FinishedAt: snap.FinishedAt,
			Error:      snap.Error,
			ResultPath: snap.ResultPath,
		}
		if status == StatusDone {
			job.Progress = 100
		}
		reg.jobs[snap.ID] = job
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("jobs: read JSONL: %w", err)
	}
	return reg, nil
}

// Concurrency returns the configured worker pool size. Useful for tests
// and for the /jobs/{id} status page header if we ever want to expose
// saturation to the UI.
func (r *Registry) Concurrency() int {
	return r.concurrency
}

// SetLogWriter attaches a JSONL writer that receives one line per job
// state change. The Registry takes ownership of closer and will close
// it when the writer is replaced or the Registry shuts down. Pass
// nil to disable persistence. Safe to call once at startup; concurrent
// calls are serialised.
func (r *Registry) SetLogWriter(w io.Writer, closer io.Closer) {
	r.logMu.Lock()
	defer r.logMu.Unlock()
	if r.logCloser != nil {
		_ = r.logCloser.Close()
	}
	r.logWriter = w
	r.logCloser = closer
}

// appendSnapshot writes one JSONL line for the given job snapshot.
// No-op when no log writer is attached.
func (r *Registry) appendSnapshot(j Job) {
	r.logMu.Lock()
	w := r.logWriter
	r.logMu.Unlock()
	if w == nil {
		return
	}
	payload, err := json.Marshal(persistedSnapshot{
		ID:         j.ID,
		Kind:       j.Kind,
		Status:     j.Status,
		Progress:   j.Progress,
		Message:    j.Message,
		StartedAt:  j.StartedAt,
		FinishedAt: j.FinishedAt,
		Error:      j.Error,
		ResultPath: j.ResultPath,
	})
	if err != nil {
		return
	}
	payload = append(payload, '\n')
	_, _ = w.Write(payload)
}

// Start queues a job of the given kind and immediately launches a
// goroutine that runs worker. The returned ID is suitable for /jobs/{id}
// routes.
func (r *Registry) Start(kind string, worker func(ctx context.Context, p *Progress) error) string {
	id := newID()
	job := &Job{
		ID:        id,
		Kind:      kind,
		Status:    StatusQueued,
		StartedAt: time.Now(),
	}
	r.mu.Lock()
	r.jobs[id] = job
	r.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	job.mu.Lock()
	job.cancelCause = cancel
	job.mu.Unlock()

	// Acquire a worker slot before launching the goroutine. If the pool
// is saturated the semaphore blocks until another worker exits, so
// the job stays in StatusQueued (set at registration) until then.
	go func() {
		r.sem <- struct{}{}
		defer func() { <-r.sem }()

		job.mu.Lock()
		// Honour a cancellation that arrived while we were queued.
		if job.cancelled {
			job.Status = StatusCancelled
			job.FinishedAt = time.Now()
			snap := cloneJob(job)
			job.mu.Unlock()
			r.appendSnapshot(snap)
			cancel()
			return
		}
		job.Status = StatusRunning
		snap := cloneJob(job)
		job.mu.Unlock()
		r.appendSnapshot(snap)

		err := worker(ctx, &Progress{job: job})

		job.mu.Lock()
		job.FinishedAt = time.Now()
		if job.cancelled {
			job.Status = StatusCancelled
		} else if err != nil {
			job.Status = StatusError
			job.Error = err.Error()
		} else {
			job.Status = StatusDone
			job.Progress = 100
		}
		snap = cloneJob(job)
		job.mu.Unlock()
		r.appendSnapshot(snap)
		cancel()
	}()

	return id
}

// cloneJob returns a value-copy of the given Job without taking its
// mutex. Callers must hold job.mu (or otherwise guarantee the Job is
// not being mutated). Used inside the Start goroutine where we
// already hold the lock and need to snapshot without re-locking.
func cloneJob(j *Job) Job {
	return Job{
		ID:         j.ID,
		Kind:       j.Kind,
		Status:     j.Status,
		Progress:   j.Progress,
		Message:    j.Message,
		StartedAt:  j.StartedAt,
		FinishedAt: j.FinishedAt,
		Error:      j.Error,
		ResultPath: j.ResultPath,
	}
}

// Get returns the snapshot for an ID and whether it exists.
func (r *Registry) Get(id string) (Job, bool) {
	r.mu.Lock()
	job, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		return Job{}, false
	}
	return job.Snapshot(), true
}

// Cancel marks the job cancelled and signals the worker via context. It
// returns ErrNotFound when no such job exists, ErrAlreadyTerminal when
// the job is already done / errored / cancelled.
var (
	ErrNotFound       = errors.New("job not found")
	ErrAlreadyTerminal = errors.New("job is already in a terminal state")
)

// DisplayLabel returns a friendly display label for the job's Kind. The
// template uses it both for the page heading and for the artifact link.
func (j Job) DisplayLabel() string {
	switch j.Kind {
	case "static_archive":
		return "Static web archive"
	case "database_pdf":
		return "Printable archive PDF"
	default:
		return j.Kind
	}
}

// SetResultPath records the saved artifact path for the given job. Safe
// to call from inside the worker or after it has completed. Workers that
// know where they wrote their output use this so the /jobs/{id}/artifact
// endpoint can stream the file back to the user. The change is also
// appended to the JSONL log if one is attached.
func (r *Registry) SetResultPath(id, path string) {
	r.mu.Lock()
	job, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		return
	}
	job.mu.Lock()
	job.ResultPath = path
	snap := cloneJob(job)
	job.mu.Unlock()
	r.appendSnapshot(snap)
}

func (r *Registry) Cancel(id string) error {
	r.mu.Lock()
	job, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	switch job.Status {
	case StatusDone, StatusError, StatusCancelled:
		return ErrAlreadyTerminal
	}
	job.cancelled = true
	if job.cancelCause != nil {
		job.cancelCause()
	}
	return nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a deterministic but unique-ish ID; the caller still
		// gets a usable value even if the system RNG fails.
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b[:])
}