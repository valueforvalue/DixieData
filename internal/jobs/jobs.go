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
	"path/filepath"
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
	registry    *Registry // set at registration so Progress can broadcast
}

// Progress is passed to a worker so it can update its job without holding
// the registry lock for the entire export.
type Progress struct {
	job *Job
}

// Set updates progress (0-100) and an optional human-readable message.
// The update is broadcast to any subscribers on the parent job
// (see Subscribe) so SSE clients see real-time progress.
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
	snap := cloneJob(p.job)
	p.job.registry.broadcast(p.job.ID, snap)
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

// NewJob constructs a Job value with the given ID and kind, leaving
// runtime fields (Status, Progress, StartedAt, etc.) zero. The mutex
// and other unexported fields are zero-initialised, so the result is
// safe to pass to read-only template rendering or to register with a
// worker via Registry.New followed by ID lookup.
//
// This constructor exists because tests in other packages cannot
// write `jobs.Job{ID: ..., Kind: ...}` literals: the mu field is
// unexported and would force tests to construct through the public
// Registry, which requires a running event loop. Tests that need a
// synthetic Job for snapshot or template rendering use NewJob.
func NewJob(id, kind string) *Job {
	return &Job{ID: id, Kind: kind}
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

	// workerWG tracks in-flight worker goroutines. Shutdown waits on
	// it after cancelling every active job so the appshell exit path
	// does not leak file handles or panic on closed channels. Each
	// worker goroutine spawned by Start does wg.Add(1) before it
	// runs and wg.Done() in its defer.
	workerWG sync.WaitGroup

	// logMu guards logWriter + logCloser. logWriter is appended to
	// on every job state change so the Registry survives a webview
	// reload or app restart. nil disables persistence.
	logMu     sync.Mutex
	logWriter io.Writer
	logCloser io.Closer

	// subMu guards subscribers. Each subscriber is a buffered chan
	// that receives a Job snapshot on every Progress.Set so the
	// /jobs/{id}/stream SSE handler can push updates in real time.
	// Slow subscribers are dropped (non-blocking send) so a wedged
	// client cannot back up the worker.
	subMu        sync.Mutex
	subscribers  map[string]map[chan Job]struct{}
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
		subscribers: map[string]map[chan Job]struct{}{},
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
			registry:   reg,
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
// No-op when no log writer is attached. The Write call runs under
// logMu so concurrent state-change events from the worker goroutine
// (Progress.Set, SetResultPath) cannot interleave bytes in the JSONL
// log. The writer itself is not assumed to be safe for concurrent use.
func (r *Registry) appendSnapshot(j Job) {
	r.logMu.Lock()
	defer r.logMu.Unlock()
	if r.logWriter == nil {
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
	_, _ = r.logWriter.Write(payload)
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
		registry:  r,
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
		r.workerWG.Add(1)
		defer r.workerWG.Done()
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
			r.broadcast(id, snap)
			cancel()
			return
		}
		job.Status = StatusRunning
		snap := cloneJob(job)
		job.mu.Unlock()
		r.appendSnapshot(snap)
		r.broadcast(id, snap)

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
		r.broadcast(id, snap)
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

// jobArtifactMimeByExt mirrors the appshell viewable-artifact map
// (issue #129) so the jobs package can decide whether a finished
// job's ResultPath is something the browser will render inline.
// Kept in sync with internal/appshell/jobs_handlers.go
// jobArtifactMimeByExt; an entry here means "open in a new tab",
// otherwise "download in the current tab".
//
// JSON stays viewable on purpose: the export-to-JSON workflow is
// developer-friendly and a developer often wants to inspect the
// output inline. The non-viewable list (.ddbak, .ddshare, .zip,
// .csv, .ics) covers the exports that are too large or binary for
// the browser to render usefully and where the old "blank tab"
// problem surfaced (issue #129).
var jobArtifactMimeByExt = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".html": "text/html; charset=utf-8",
	".htm":  "text/html; charset=utf-8",
	".txt":  "text/plain; charset=utf-8",
	".json": "application/json; charset=utf-8",
}

// IsViewableArtifact reports whether the job's ResultPath is a file
// the browser will render inline (PDF, image, HTML, plain text,
// JSON). The /jobs/{id} status page uses this to choose between
// target="_blank" (viewable: open in a new tab) and the download
// attribute (non-viewable: trigger a save dialog in the current
// tab so the user never sees a blank tab). Returns false when the
// job has no ResultPath yet or the extension is unknown.
func (j Job) IsViewableArtifact() bool {
	if j.ResultPath == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(j.ResultPath))
	_, ok := jobArtifactMimeByExt[ext]
	return ok
}

// ArtifactFilename returns the base name of the job's ResultPath
// (e.g. "june-2026.ddbak"). Used by the status page as the
// `download` attribute on non-viewable artifacts so the browser
// saves the file with the correct name instead of the long
// `/jobs/{id}/artifact` URL path.
func (j Job) ArtifactFilename() string {
	if j.ResultPath == "" {
		return ""
	}
	return filepath.Base(j.ResultPath)
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
	r.broadcast(id, snap)
}

// MostRecentActive returns the most recently started job that is
// still queued or running, or nil if none. Used by the layout
// progress slot to render whichever background task the user kicked
// off most recently regardless of which page they are on. Returns a
// value-copy so callers can read fields without holding any lock.
func (r *Registry) MostRecentActive() *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *Job
	for _, j := range r.jobs {
		if j.Status != StatusQueued && j.Status != StatusRunning {
			continue
		}
		snap := cloneJob(j)
		if latest == nil || snap.StartedAt.After(latest.StartedAt) {
			latest = &snap
		}
	}
	return latest
}

// Shutdown cancels every running/queued job and waits for the worker
// goroutines to drain. Bounded by ctx; if the deadline expires before
// the workers exit, Shutdown returns ctx.Err() and the goroutines are
// abandoned (they will eventually finish on their own unless blocked
// on I/O). Called from the appshell shutdown sequence so file handles
// held by export workers are released before main returns. The WJ-2
// appendSnapshot race fix in 271149a made file-handle ownership
// explicit; this method is the matching exit-side guarantee that
// those handles are actually released.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	for _, j := range r.jobs {
		if j.Status == StatusQueued || j.Status == StatusRunning {
			j.cancelCause()
		}
	}
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.workerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Subscribe registers a buffered channel as a listener for snapshot
// updates on the given job. Returns nil if the job is unknown so
// callers can pass user input without a separate existence check
// without leaking an orphan subscriber entry.
//
// The buffer size keeps one or two slow events in flight without
// blocking the broadcaster; a wedged subscriber is silently dropped
// to protect the worker.
//
// Callers MUST call Unsubscribe(id, ch) when they stop reading so the
// registry can garbage-collect the channel.
func (r *Registry) Subscribe(id string) chan Job {
	r.mu.Lock()
	_, exists := r.jobs[id]
	r.mu.Unlock()
	if !exists {
		return nil
	}
	ch := make(chan Job, 8)
	r.subMu.Lock()
	if r.subscribers == nil {
		r.subMu.Unlock()
		return ch
	}
	subs, ok := r.subscribers[id]
	if !ok {
		subs = map[chan Job]struct{}{}
		r.subscribers[id] = subs
	}
	subs[ch] = struct{}{}
	r.subMu.Unlock()

	// Push the current snapshot immediately so subscribers don't have
	// to wait for the next Progress.Set to see something.
	if snap, ok := r.Get(id); ok {
		select {
		case ch <- snap:
		default:
		}
	}
	return ch
}

// Unsubscribe removes a previously-registered channel and closes it.
// Safe to call with an unknown id or channel; no-op in those cases.
func (r *Registry) Unsubscribe(id string, ch chan Job) {
	r.subMu.Lock()
	if subs, ok := r.subscribers[id]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(r.subscribers, id)
		}
	}
	r.subMu.Unlock()
	select {
	case _, ok := <-ch:
		// drain any pending snapshot so the close doesn't race a send
		_ = ok
	default:
	}
	close(ch)
}

// broadcast sends a snapshot to every subscriber for the given job.
// The send is non-blocking; a slow subscriber is skipped this round
// rather than backing up the worker goroutine.
func (r *Registry) broadcast(id string, snap Job) {
	r.subMu.Lock()
	subs := r.subscribers[id]
	chans := make([]chan Job, 0, len(subs))
	for ch := range subs {
		chans = append(chans, ch)
	}
	r.subMu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- snap:
		default:
		}
	}
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