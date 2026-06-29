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
	"os"
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

// SilentKinds enumerates job kinds that MUST NOT surface in the
// global layout progress popup. The popup is meant for tasks the
// user would otherwise lose track of while navigating the app
// (long-running PDF / ddbak exports, calendar imports, etc).
// Kinds in this set are still tracked, still poll-able via
// /jobs/{id}, and still land on /jobs/{id} via the standard 303
// — they just do not render the floating card. Use this for
// jobs whose export path is so short and whose destination
// page (/jobs/{id}) already serves as the landing.
//
// Add a kind here only when:
//   1. The worker's destination page (always /jobs/{id}) is
//      self-sufficient — the user does not need the popup
//      to remember where they were going.
//   2. The artifact (if any) does not preview well in a new
//      tab, so the popup's "Open result" button would be a
//      dead end.
var SilentKinds = map[string]struct{}{
	"static_archive": {},
}

// IsSilentKind reports whether the given job kind opts out of
// the global layout progress popup.
func IsSilentKind(kind string) bool {
	_, ok := SilentKinds[kind]
	return ok
}

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
	Result      JobResult
	mu          sync.Mutex
	cancelled   bool
	cancelCause context.CancelFunc
	registry    *Registry// set at registration so Progress can broadcast
}

// JobResult is the worker-supplied completion payload. Populated
// by Registry.SetResult before the worker returns nil so /jobs/{id}
// can render per-kind stats on the terminal summary card:
//
//   - Exports fill Records / Images / Sources.
//   - Shared imports fill Added / Merged / Skipped / Conflicts /
//     SourcesImported / ImagesImported.
//   - Memorial JSON imports fill Added / Skipped / Failed (the
//     preview-then-confirm flow does not stage Merge Review).
//   - Backup restore fills ReplacedRecords / ReplacedImages plus
//     BackupSchema / CurrentSchema / MigrationRan.
//
// The struct is intentionally a single value with optional fields
// rather than a discriminated union so callers from different
// kinds can share one setter and one storage slot on Job.
// Fields default to zero; Summary() renders a stat line only
// when the corresponding field is > 0 (or true for MigrationRan),
// so legacy kinds that don't fill the struct are unaffected.
type JobResult struct {
	// Path is promoted to Job.ResultPath on SetResult so the
	// /jobs/{id}/artifact endpoint still streams the saved file
	// when the worker forgets to call SetResultPath explicitly.
	Path string

	// Export counts.
	Records int // Person Records written to the artifact
	Images  int // Image files copied into the artifact
	Sources int // Source Records (claims + findings) included

	// Shared-import counts.
	Added           int // Person Records inserted (new from incoming)
	Merged          int // Person Records updated (matched + changed)
	Skipped         int // Person Records unchanged (matched + same)
	Conflicts       int // Staged for Merge Review (>= 1 means visit /merge-review/{id})
	ImagesImported  int
	SourcesImported int

	// Memorial JSON import counts (preview-then-confirm flow).
	Failed int // Memorial records that could not be imported

	// Backup restore (replace semantics, not merge).
	ReplacedRecords int
	ReplacedImages  int
	BackupSchema    int // schema version of the .ddbak
	CurrentSchema   int // schema version DixieData is on now
	MigrationRan    bool

	// LogPath is an optional companion artifact (e.g. memorial
	// import error log) that the summary card can offer as a
	// secondary action. Distinct from Path so the primary
	// artifact keeps a single download link.
	LogPath string
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

// Shimmer animates the progress bar so the user sees continuous
// motion during long-running exports where the worker doesn't
// have natural sub-step granularity to report (e.g. a single
// ExportJSONWithStats call doesn't know how much of the encode
// pass is done). Without Shimmer the bar jumps from the worker's
// last Set() (typically 5 or 20) straight to 100 when the work
// completes, which the user reports as 'the progress bar never
// moves'.
//
// Shimmer walks Progress monotonically from `from` toward `to-1`,
// one tick every 250ms, for the duration the worker is busy.
// The worker is expected to call Shimmer right after its last
// pre-work Set() (e.g. p.Set(20, 'Writing JSON'); go p.Shimmer(ctx,
// 20, 95, 30*time.Second)) and then immediately do the real work.
// Real-progress calls (worker p.Set(100, 'Done')) win the last
// write — Shimmer honours the lock and never overruns a higher
// value.
//
// The `to` parameter should be < 100 so the worker reserves the
// final 100 signal for itself. Pass 95 for most exports; pass a
// lower value when there are multiple discrete sub-steps the
// worker wants the user to see (e.g. 60 leaves 35 points of head
// room for 'Finalising archive' / 'Writing preview' / etc.).
//
// Shimmer exits early when ctx is cancelled (worker shutdown).
func (p *Progress) Shimmer(ctx context.Context, from, to int, duration time.Duration, message string) {
	if p == nil || p.job == nil {
		return
	}
	if from < 0 {
		from = 0
	}
	if to > 99 {
		to = 99
	}
	if to <= from+1 {
		// No room to walk (to must be >= from+2 for at least one
		// increment). Worker should just Set(100, ...) at the end.
		return
	}
	if duration <= 0 {
		duration = 30 * time.Second
	}
	// Number of integer steps to advance from `from` to `to-1`.
	steps := to - 1 - from
	// Per-step interval = duration / steps, clamped to a sensible
	// range (250ms is the polling cadence floor; >2.5s feels sticky).
	stepInterval := duration / time.Duration(steps)
	if stepInterval < 250*time.Millisecond {
		stepInterval = 250 * time.Millisecond
	}
	if stepInterval > 2*time.Second {
		// Long-running exports step too slowly. Cap so the bar
		// always makes visible motion every ~2s.
		stepInterval = 2 * time.Second
		duration = stepInterval * time.Duration(steps)
	}
	go func() {
		timer := time.NewTimer(stepInterval)
		defer timer.Stop()
		current := from
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				p.job.mu.Lock()
				// Never walk downward. If the worker called
				// Set() with a higher value between our ticks,
				// leave it there and exit so we don't fight
				// the worker (a Shimmer that overshoots would
				// be visible as a progress bar going
				// backwards).
				if p.job.Progress > current {
					p.job.mu.Unlock()
					return
				}
				if current < to-1 {
					current++
				}
				p.job.Progress = current
				if message != "" {
					p.job.Message = message
				}
				snap := cloneJob(p.job)
				p.job.registry.broadcast(p.job.ID, snap)
				p.job.mu.Unlock()
				if current >= to-1 {
					return
				}
				timer.Reset(stepInterval)
			}
		}
	}()
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
		Result:     j.Result,
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
	// Result is omitempty so older log files (written before
	// stats landed) parse cleanly. NewFromLog drops the field
	// when absent; live jobs always carry a zero JobResult.
	Result JobResult `json:"result,omitempty"`
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
			Result:     snap.Result,
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
		Result:     j.Result,
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
		Result:     j.Result,
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

// JobSummary is the structured payload the /jobs/{id} status page
// renders in its summary card. The fields are intentionally
// kind-agnostic so the template can render any job without a
// per-kind switch; jobs that do not surface a particular datum
// (soldier count for a JSON export, image count for a static
// archive) leave it zero. Issue #131's acceptance criteria
// require counts + size + duration, so every terminal-state
// summary card carries those three at minimum.
type JobSummary struct {
	Kind        string
	Label       string
	Headline    string
	DetailLines []string
	SizeBytes   int64
	Duration    time.Duration
	ResultPath  string
}

// Summary returns a JobSummary describing the job's terminal
// state. Headline + DetailLines are the user-facing copy that
// the template renders in the summary card. ResultPath is
// always populated for finished jobs so the card can name the
// on-disk file even when the artifact is not viewable in the
// browser (issue #129 + #131). Returns a zero-value summary
// for jobs that are still running.
func (j Job) Summary() JobSummary {
	s := JobSummary{
		Kind:       j.Kind,
		Label:      j.DisplayLabel(),
		ResultPath: j.ResultPath,
	}
	if j.Status != StatusDone || j.StartedAt.IsZero() || j.FinishedAt.IsZero() {
		return s
	}
	s.Duration = j.FinishedAt.Sub(j.StartedAt).Round(time.Second)
	if j.ResultPath != "" {
		if info, err := os.Stat(j.ResultPath); err == nil {
			s.SizeBytes = info.Size()
		}
	}
	switch j.Kind {
	case "soldier_pdf", "soldier_pdf_no_images":
		s.Headline = fmt.Sprintf("%s complete — %s.", j.DisplayLabel(), formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
		}
	case "soldier_jpg":
		s.Headline = fmt.Sprintf("Soldier JPG export complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
		}
	case "monthly_pdf":
		s.Headline = fmt.Sprintf("Monthly calendar PDF complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
		}
	case "backup_archive":
		s.Headline = fmt.Sprintf("Backup archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
			"Use 'Load Backup' on the Share page to restore this archive.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "shared_archive":
		s.Headline = fmt.Sprintf("Shared archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
			"Send this .ddshare file to another DixieData user; they can preview it on the Share page.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "json_export", "excel_export", "icalendar_export":
		s.Headline = fmt.Sprintf("%s complete — %s.", j.DisplayLabel(), formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "database_pdf":
		s.Headline = fmt.Sprintf("Printable archive PDF complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
			"The PDF contains every record grouped and sorted per your export settings.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "static_archive":
		s.Headline = fmt.Sprintf("Static archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
			"Open the .zip and host it on any static-file web server to browse the archive without DixieData.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "insights_pdf", "bug_report":
		s.Headline = fmt.Sprintf("%s complete — %s.", j.DisplayLabel(), formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
		}
	case "image_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", s.Duration)}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", s.Duration)}
		}
	case "backup_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", s.Duration)}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", s.Duration)}
		}
		s.DetailLines = appendBackupRestoreStats(s.DetailLines, j.Result)
	case "shared_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", s.Duration)}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", s.Duration)}
		}
		s.DetailLines = appendSharedImportStats(s.DetailLines, j.Result)
	case "memorial_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		s.DetailLines = []string{fmt.Sprintf("Duration: %s", s.Duration)}
		s.DetailLines = appendMemorialImportStats(s.DetailLines, j.Result)
	default:
		s.Headline = fmt.Sprintf("%s complete — %s.", j.DisplayLabel(), formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", s.Duration),
		}
	}
	return s
}

// appendExportStats conditionally appends records / images /
// sources lines to the summary card based on which fields the
// worker populated. Lines appear only when the count is > 0 so
// legacy workers (and kinds that do not surface stats, like
// insights_pdf) keep their existing copy unchanged.
//
// Ordering: Records before Images before Sources, matching the
// order the user thinks about the artifact (people → media →
// evidence). The Size and Duration lines are already on the card
// before this helper runs.
func appendExportStats(lines []string, r JobResult) []string {
	if r.Records > 0 {
		lines = append(lines, fmt.Sprintf("Person records: %d", r.Records))
	}
	if r.Images > 0 {
		lines = append(lines, fmt.Sprintf("Images: %d", r.Images))
	}
	if r.Sources > 0 {
		lines = append(lines, fmt.Sprintf("Source records: %d", r.Sources))
	}
	return lines
}

// appendSharedImportStats renders the merge-review headline
// (Added / Merged / Skipped / Conflicts) plus images / sources.
// When Conflicts > 0 the user is reminded to open Merge Review;
// when 0 the import is fully resolved.
func appendSharedImportStats(lines []string, r JobResult) []string {
	if r.Added > 0 || r.Merged > 0 || r.Skipped > 0 {
		lines = append(lines, fmt.Sprintf("Person records: %d added, %d merged, %d skipped",
			r.Added, r.Merged, r.Skipped))
	}
	if r.Conflicts > 0 {
		lines = append(lines, fmt.Sprintf("Conflicts staged for review: %d — see Merge Review below.", r.Conflicts))
	}
	if r.ImagesImported > 0 {
		lines = append(lines, fmt.Sprintf("Images imported: %d", r.ImagesImported))
	}
	if r.SourcesImported > 0 {
		lines = append(lines, fmt.Sprintf("Source records imported: %d", r.SourcesImported))
	}
	return lines
}

// appendMemorialImportStats renders the dry-run-then-confirm
// import counts. Memorial JSON is additive (no Merge Review), so
// the headline is Added / Skipped / Failed. The optional error log
// at Result.LogPath becomes a secondary download action on the
// summary card (see jobs.templ::jobSummaryCard).
func appendMemorialImportStats(lines []string, r JobResult) []string {
	if r.Added > 0 || r.Skipped > 0 || r.Failed > 0 {
		lines = append(lines, fmt.Sprintf("Person records: %d added, %d skipped, %d failed",
			r.Added, r.Skipped, r.Failed))
	}
	if r.ImagesImported > 0 {
		lines = append(lines, fmt.Sprintf("Images imported: %d", r.ImagesImported))
	}
	return lines
}

// appendBackupRestoreStats renders the replace-semantics summary:
// how many records/images the backup overwrote and whether the
// schema migration ran. The schema line is always shown (even when
// migration did not run) because schema parity is the headline
// question after a full restore.
func appendBackupRestoreStats(lines []string, r JobResult) []string {
	if r.ReplacedRecords > 0 || r.ReplacedImages > 0 {
		lines = append(lines, fmt.Sprintf("Replaced: %d records, %d images", r.ReplacedRecords, r.ReplacedImages))
	}
	if r.BackupSchema > 0 || r.CurrentSchema > 0 {
		if r.MigrationRan {
			lines = append(lines, fmt.Sprintf("Schema migrated: backup v%d → current v%d", r.BackupSchema, r.CurrentSchema))
		} else {
			lines = append(lines, fmt.Sprintf("Schema: backup v%d = current v%d (no migration)", r.BackupSchema, r.CurrentSchema))
		}
	}
	return lines
}

// formatBytes renders a byte count as a human-friendly size
// (e.g. 1.2 MB, 489 kB). Used by JobSummary.DetailLines so the
// summary card stays readable without a separate helper import.
func formatBytes(n int64) string {
	const (
		kB = 1024
		mB = 1024 * kB
		gB = 1024 * mB
	)
	switch {
	case n >= gB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gB))
	case n >= mB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mB))
	case n >= kB:
		return fmt.Sprintf("%.1f kB", float64(n)/float64(kB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// jobArtifactMimeByExt mirrors the appshell viewable-artifact map
// (issue #129) so the jobs package can decide whether a finished
// job's ResultPath is something the browser will render inline.
// Kept in sync with internal/appshell/jobs_handlers.go
// jobArtifactMimeByExt; an entry here means the artifact endpoint
// can serve the file with Content-Disposition: inline, otherwise
// the endpoint sets Content-Disposition: attachment.
//
// The /jobs/{id} status page no longer renders an "Open {label}"
// button that points at the artifact endpoint (a previous version
// did this and produced the "blank tab after Open result"
// complaint). IsViewableArtifact is preserved because the
// artifact endpoint still uses it to choose a disposition.
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
// the artifact endpoint will serve inline (PDF, image, HTML, text,
// JSON). The endpoint uses this to choose between inline and
// attachment disposition. Returns false when the job has no
// ResultPath yet or the extension is unknown.
func (j Job) IsViewableArtifact() bool {
	if j.ResultPath == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(j.ResultPath))
	_, ok := jobArtifactMimeByExt[ext]
	return ok
}

// DismissTargetPath returns the in-app path the user lands on
// when they dismiss the /jobs/{id} status page. Most exports
// were kicked off from the Share page; imports were kicked off
// from Share too (via the Load Backup / Preview Memorial JSON
// buttons); single-record exports are routed back to that
// soldier. Issue #131 prefers the referring page, but the
// /jobs/{id} status page does not always have the original
// referer, so the template falls back to this kind-specific
// path when no referer was saved.
func (j Job) DismissTargetPath() string {
	switch j.Kind {
	case "image_import":
		// Image imports are per-soldier; we don't know the
		// soldier id from the job alone, so fall back to the
		// browse page where the user can pick the soldier
		// again.
		return "/browse"
	case "backup_import":
		return "/share"
	case "shared_import", "shared_archive":
		return "/share"
	case "monthly_pdf":
		return "/calendar"
	case "soldier_pdf", "soldier_pdf_no_images", "soldier_jpg":
		return "/soldiers"
	case "insights_pdf":
		return "/insights"
	default:
		return "/share"
	}
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

// SetResult records the worker-supplied completion payload for the
// given job. Safe to call from inside the worker before it returns
// nil, or from another goroutine after the worker has completed
// (the job entry stays in the Registry until Shutdown). If the
// payload carries a non-empty Path, it is also written to the job's
// ResultPath so /jobs/{id}/artifact streams the artifact without
// callers having to call SetResultPath separately.
//
// The change is appended to the JSONL log if one is attached, and
// the snapshot is broadcast to any SSE subscribers so live progress
// pages reflect the final stats without a page reload.
func (r *Registry) SetResult(id string, result JobResult) {
	r.mu.Lock()
	job, ok := r.jobs[id]
	r.mu.Unlock()
	if !ok {
		return
	}
	job.mu.Lock()
	if result.Path != "" {
		job.ResultPath = result.Path
	}
	job.Result = result
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
//
// Jobs whose Kind appears in SilentKinds are excluded: their
// /jobs/{id} status page is the landing, so the floating popup
// card would only get in the way (especially when the artifact
// is a binary that does not preview well in a new tab).
func (r *Registry) MostRecentActive() *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *Job
	for _, j := range r.jobs {
		if j.Status != StatusQueued && j.Status != StatusRunning {
			continue
		}
		if IsSilentKind(j.Kind) {
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