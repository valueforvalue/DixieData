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
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Status values.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusDone      = "done"
	StatusError     = "error"
	StatusCancelled = "cancelled"
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

// Concurrency returns the configured worker pool size. Useful for tests
// and for the /jobs/{id} status page header if we ever want to expose
// saturation to the UI.
func (r *Registry) Concurrency() int {
	return r.concurrency
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
			job.mu.Unlock()
			cancel()
			return
		}
		job.Status = StatusRunning
		job.mu.Unlock()

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
		job.mu.Unlock()
		cancel()
	}()

	return id
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
// endpoint can stream the file back to the user.
func (r *Registry) SetResultPath(id, path string) {
	r.mu.Lock()
	job, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		return
	}
	job.mu.Lock()
	job.ResultPath = path
	job.mu.Unlock()
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