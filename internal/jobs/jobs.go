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

// Registry holds the live jobs for a process.
type Registry struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{jobs: map[string]*Job{}}
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

	go func() {
		job.mu.Lock()
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