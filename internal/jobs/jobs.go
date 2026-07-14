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
	"sort"
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
	ID                    string
	Kind                  string
	Status                string
	Progress              int
	Message               string
	StartedAt             time.Time
	FinishedAt            time.Time
	Error                 string
	ResultPath            string
	Result                JobResult
	AwaitingConfirmation  bool // true when StartManual registered this job; /jobs/{id} renders a Confirm/Cancel card
	mu                    sync.Mutex
	cancelled             bool
	cancelCause           context.CancelFunc
	registry              *Registry// set at registration so Progress can broadcast
}

// JobResult is the worker-supplied completion payload. Populated
// by Registry.SetResult before the worker returns nil so /jobs/{id}
// can render per-kind stats on the terminal summary card:
//
//   - Exports fill Records / Images / Sources (and StaticArchive
//     for HTML archive exports, issue #492).
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

	// Issue #492: HTML archive export contents (static_archive
	// job kind). Captured at export time by the worker so the
	// /jobs/{id} status page can show the user what was
	// exported without unzipping the artifact. The kind-specific
	// fields below give the GUI status page + the CLI jobs
	// show {id} command a category breakdown that mirrors the
	// GUI's "Archive contents" panel.
	StaticArchive *StaticArchiveResult

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

	// Issue #552: Google Drive / Google Sheets uploads complete
	// with a service-side result the worker was discarding before
	// this fix. RemoteURL captures the user-facing link (the
	// WebViewLink the Drive API returns, with a Sheets-flavoured
	// fallback synthesised by googleDriveUploadResult when Drive
	// omits it for spreadsheet files), RemoteName is the
	// user-visible file name on the remote, and RemoteKind picks
	// the button label rendered on the summary card ("Open in
	// Drive" vs "Open in Sheets"). RemoteKind is a free-form
	// string ("drive" / "sheets") rather than a typed enum so
	// future integration shapes don't need a code change to land;
	// Job.Summary() is the single switch that maps the value to
	// a UI label. All three are omitempty so pre-#552 entries in
	// the JSONL log parse cleanly into the zero JobResult.
	RemoteURL  string `json:"remote_url,omitempty"`
	RemoteName string `json:"remote_name,omitempty"`
	RemoteKind string `json:"remote_kind,omitempty"`

	// Issue #556 slice 1: free-form per-kind result field for the
	// image_orphan_cleanup summarizer (slice 3 reads this to render
	// "Trash root: <path>" in the summary card). Empty for every
	// other kind. Other zero-state kinds (review_bulk_resolve,
	// duplicate_audit, etc.) anchor on j.Message and don't need a
	// new field — adding per-kind counters is out of scope.
	TrashRoot string `json:"trash_root,omitempty"`
}

// StaticArchiveResult is the per-kind export snapshot for the
// static_archive job (issue #492). All counts are computed at
// export time by walking the data the worker actually wrote
// into the .zip, not by re-querying the DB at view time (which
// would drift if the user edits records between export and
// status-page view).
//
// Pointer-typed on JobResult so the zero-value
// `JobResult{}` stays nil-safe — Summary() and the CLI jobs
// commands nil-check before rendering. JSONL round-trips
// cleanly because encoding/json handles nil pointers as
// `null` and rehydrates the absence on read.
type StaticArchiveResult struct {
	PersonRecords   int // Total Person Records exported (soldier + wife/widow + linked_person)
	SpouseRecords   int // Of the Person Records, the spouse (wife/widow) subset
	LinkedPeople    int // Of the Person Records, the linked_person subset
	Events          int // Event Records included in archive_data.js
	Articles        int // Articles included in archive_data.js
	PersonImages    int // Person Record image files copied into images/
	SourceRecords   int // Source Records (claims + findings) attached to Person Records
	DistinctTags    int // Distinct tag names referenced by any exported Person Record
	// Issue #498 slice 5: extra per-page counts surfaced on the
	// /jobs/{id} summary card so the user sees the Calendar + Insights
	// page contents alongside the per-record counts.
	CalendarDaysWithData int // Days with at least one anniversary/event/holiday marker (Calendar landing)
	InsightsSections     int // Insights dimensions with non-empty counts (max 7: cemetery_density, confederate_home_status, pension_distribution, unit_representation, birth_decade_distribution, death_decade_distribution, record_types — always counted)
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
		ID:                   j.ID,
		Kind:                 j.Kind,
		Status:               j.Status,
		Progress:             j.Progress,
		Message:              j.Message,
		StartedAt:            j.StartedAt,
		FinishedAt:           j.FinishedAt,
		Error:                j.Error,
		ResultPath:           j.ResultPath,
		Result:               j.Result,
		AwaitingConfirmation: j.AwaitingConfirmation,
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

	// shutdownOnce serialises the Shutdown Wait goroutine so the
	// second-and-later callers attach to the same done channel
	// instead of racing a fresh WaitGroup.Wait against a late Add.
	// Without this, a test that calls Shutdown once explicitly and
	// again from t.Cleanup trips Go's
	// 'WaitGroup is reused before previous Wait has returned'
	// runtime check.
	shutdownOnce sync.Once

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
// workerWG.Add must land BEFORE the goroutine is spawned so a
// concurrent Shutdown() can never observe WaitGroup counter == 0
// followed by a fresh Add (which Go's sync runtime rejects with
// 'WaitGroup is reused before previous Wait has returned').
	r.workerWG.Add(1)
	go func() {
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

// StartManual registers a job that sits in StatusQueued + Progress=0
// until someone explicitly releases it. The worker runs only after
// the caller invokes the returned release() function. Used for
// imports that need user confirmation on /jobs/{id} before any
// irreversible change is made (Memorial JSON today; could be
// re-used for any destructive import in the future).
//
// Behaviour:
//   - The job is created with StatusQueued + Progress=0. Anything
//     the caller has already Set() on the supplied Progress (e.g.
//     a pre-computed preview summary) is reflected in the queued
//     snapshot.
//   - Calling release() flips the job to StatusRunning and starts
//     the worker goroutine. Calling cancel() flips it to
//     StatusCancelled and never runs the worker.
//   - If neither release() nor cancel() is called, the job stays
//     in StatusQueued indefinitely. The poll fragment treats the
//     queued state as terminal-pinning (no hx-trigger), so the
//     /jobs/{id} page renders a static Awaiting Confirmation
//     card with Confirm + Cancel buttons.
//   - Both release() and cancel() return ErrAlreadyTerminal if
//     the job has already moved on (e.g. the registry was
//     shut down).
//
// Thread-safety: the returned callbacks may be invoked from any
// goroutine and at most once.
func (r *Registry) StartManual(kind string, initialMessage string, worker func(ctx context.Context, p *Progress) error) (id string, release func() error, cancel func() error) {
	id = newID()
	job := &Job{
		ID:                   id,
		Kind:                 kind,
		Status:               StatusQueued,
		StartedAt:            time.Now(),
		registry:             r,
		Message:              initialMessage,
		AwaitingConfirmation: true,
	}
	r.mu.Lock()
	r.jobs[id] = job
	r.mu.Unlock()

	// Track which callback was invoked so release and cancel are
	// exactly-once. done guards the single transition; the
	// release channel unblocks the worker goroutine.
	done := make(chan struct{}, 1)
	released := false
	cancelled := false

	ctx, ctxCancel := context.WithCancel(context.Background())
	job.mu.Lock()
	job.cancelCause = ctxCancel
	job.mu.Unlock()

	// Run the worker goroutine. It blocks on the release channel
	// (or ctx if cancelled) before doing anything visible. The
	// worker is responsible for honouring ctx so cancel() unblocks
	// it cleanly. workerWG.Add must land BEFORE the goroutine is
	// spawned to keep Shutdown's Wait safe (see Start above for the
	// 'WaitGroup reused' rationale).
	r.workerWG.Add(1)
	go func() {
		defer r.workerWG.Done()

		select {
		case <-done:
			// Release was called; fall through to running.
		case <-ctx.Done():
			// Cancel was called before release. Leave the job
			// in StatusCancelled (Start set it before we were
			// told to run) and exit. broadcast already fired
			// when Cancel set the status.
			ctxCancel()
			return
		}

		if cancelled {
			// Released but cancel was called between release and
			// the worker actually starting. Treat as cancelled.
			job.mu.Lock()
			job.Status = StatusCancelled
			job.FinishedAt = time.Now()
			snap := cloneJob(job)
			job.mu.Unlock()
			r.appendSnapshot(snap)
			r.broadcast(id, snap)
			ctxCancel()
			return
		}
		released = true

		job.mu.Lock()
		job.Status = StatusRunning
		// Clear the awaiting-confirmation flag so the /jobs/{id}
		// page transitions cleanly from the confirmation card to
		// the standard progress widget once the worker starts.
		job.AwaitingConfirmation = false
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
		ctxCancel()
	}()

	release = func() error {
		job.mu.Lock()
		defer job.mu.Unlock()
		if released || cancelled {
			return ErrAlreadyTerminal
		}
		if job.cancelled {
			return ErrAlreadyTerminal
		}
		// Push the release signal; the worker goroutine consumes
		// it and starts the worker.
		select {
		case done <- struct{}{}:
		default:
			// Should not happen with our single-send pattern.
		}
		return nil
	}
	cancel = func() error {
		job.mu.Lock()
		if released || cancelled {
			job.mu.Unlock()
			return ErrAlreadyTerminal
		}
		cancelled = true
		job.cancelled = true
		job.Status = StatusCancelled
		job.FinishedAt = time.Now()
		snap := cloneJob(job)
		job.mu.Unlock()
		r.appendSnapshot(snap)
		r.broadcast(id, snap)
		// Cancel the worker's ctx so any goroutine listening on
		// it can exit cleanly. The worker goroutine itself is
		// blocked on `done`; it'll wake up via ctx.Done and
		// exit before invoking fn.
		ctxCancel()
		return nil
	}
	return id, release, cancel
}

// cloneJob returns a value-copy of the given Job without taking its
// mutex. Callers must hold job.mu (or otherwise guarantee the Job is
// not being mutated). Used inside the Start goroutine where we
// already hold the lock and need to snapshot without re-locking.
func cloneJob(j *Job) Job {
	return Job{
		ID:                   j.ID,
		Kind:                 j.Kind,
		Status:               j.Status,
		Progress:             j.Progress,
		Message:              j.Message,
		StartedAt:            j.StartedAt,
		FinishedAt:           j.FinishedAt,
		Error:                j.Error,
		ResultPath:           j.ResultPath,
		Result:               j.Result,
		AwaitingConfirmation: j.AwaitingConfirmation,
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

// RecentJobs returns up to n terminal-state jobs sorted by
// StartedAt descending. Used by the /share landing's "Recent
// activity" section (issue #265). Terminal states are
// StatusDone, StatusError, StatusCancelled, and
// StatusInterrupted; queued + running jobs are excluded
// because they show in the global jobs-progress overlay
// instead. n <= 0 returns an empty slice.
//
// Cost: O(jobs * log jobs) where jobs is the in-memory
// registry size. The registry is bounded by the user's
// session (jobs that have been rehydrated from the JSONL
// log on startup); the in-memory map holds the full
// history for the running session, not just the last n.
// For a researcher with ~hundreds of jobs over a year,
// this is well under a millisecond.
func (r *Registry) RecentJobs(n int) []Job {
	if n <= 0 {
		return []Job{}
	}
	r.mu.Lock()
	out := make([]Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		snap := job.Snapshot()
		switch snap.Status {
		case StatusDone, StatusError, StatusCancelled, StatusInterrupted:
			out = append(out, snap)
		}
	}
	r.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.After(out[j].StartedAt)
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// Cancel marks the job cancelled and signals the worker via context. It
// returns ErrNotFound when no such job exists, ErrAlreadyTerminal when
// the job is already done / errored / cancelled.
var (
	ErrNotFound       = errors.New("job not found")
	ErrAlreadyTerminal = errors.New("job is already in a terminal state")
)

// DisplayLabel returns a friendly display label for the job's Kind.
// Issue #556 slice 1: reads from KindRegistry. The legacy 2-entry
// switch (static_archive + database_pdf) is replaced with a full
// 25-entry registry. Unknown kinds (legacy JSONL log entries that
// pre-date the registry) get humanizeKind(kind) so the UI never
// surfaces raw snake_case. See also `FailedVerb` (jobverbs.go)
// for the kind → error/cancel verb mapping; future slices migrate
// that helper onto the same registry.
func (j Job) DisplayLabel() string {
	if meta, ok := KindRegistry[j.Kind]; ok && meta.DisplayLabel != "" {
		return meta.DisplayLabel
	}
	return humanizeKind(j.Kind)
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

	// Issue #552: remote-link affordance for jobs whose
	// side-effect lives on a third-party service (Google
	// Drive, Google Sheets). RemoteURL is the user-facing
	// link the worker captured from the upload's service-side
	// result; RemoteLabel is the button text the summary
	// card renders ("Open in Drive" / "Open in Sheets").
	// Both empty means no remote link — the template skips
	// the button. Kept on JobSummary rather than DetailLines
	// so the template can render an <a> anchor (matching the
	// Download log + Copy path patterns) instead of a
	// detect-and-rewritten magic string.
	RemoteURL   string
	RemoteLabel string
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
	// Issue #543: keep the raw sub-second precision so
	// formatDuration can render "0.8s" for fast jobs instead
	// of collapsing them to "Duration: 0s". The old
	// .Round(time.Second) made the duration line useless for
	// cleanup / audit / review-bulk jobs that finish in
	// hundreds of milliseconds.
	s.Duration = j.FinishedAt.Sub(j.StartedAt)
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
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	case "soldier_jpg":
		s.Headline = fmt.Sprintf("Soldier JPG export complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	case "monthly_pdf":
		s.Headline = fmt.Sprintf("Monthly calendar PDF complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	case "backup_archive":
		s.Headline = fmt.Sprintf("Backup archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Use 'Load Backup' on the Share page to restore this archive.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "shared_archive":
		s.Headline = fmt.Sprintf("Shared archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Send this .ddshare file to another DixieData user; they can preview it on the Share page.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "shared_archive_subset":
		s.Headline = fmt.Sprintf("Subset shared archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Subset of Person Records staged from the Share Queue; send to another DixieData user.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "json_export", "excel_export", "icalendar_export":
		s.Headline = fmt.Sprintf("%s complete — %s.", j.DisplayLabel(), formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "database_pdf":
		s.Headline = fmt.Sprintf("Printable archive PDF complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"The PDF contains every record grouped and sorted per your export settings.",
		}
		s.DetailLines = appendExportStats(s.DetailLines, j.Result)
	case "static_archive":
		s.Headline = fmt.Sprintf("Static archive complete — %s.", formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
			"Open the .zip and host it on any static-file web server to browse the archive without DixieData.",
		}
		// Issue #492: per-kind content counts (Person Records, Events,
		// Articles, etc.) when the worker populated them. Old jobs
		// persisted in the JSONL log before this slice show the
		// static fallback line (D6 decision).
		if j.Result.StaticArchive != nil {
			s.DetailLines = appendStaticArchiveStats(s.DetailLines, *j.Result.StaticArchive)
		} else {
			s.DetailLines = append(s.DetailLines, "Contents unavailable for this archive — exported before counts were tracked.")
		}
	case "insights_pdf", "bug_report":
		s.Headline = fmt.Sprintf("%s complete — %s.", j.DisplayLabel(), formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
		}
	case "image_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		}
	case "backup_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		}
		s.DetailLines = appendBackupRestoreStats(s.DetailLines, j.Result)
	case "shared_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		if j.Message != "" {
			s.DetailLines = []string{j.Message, fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		} else {
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		}
		s.DetailLines = appendSharedImportStats(s.DetailLines, j.Result)
	case "memorial_import":
		s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
		s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		s.DetailLines = appendMemorialImportStats(s.DetailLines, j.Result)
	// Issue #543: zero-state kinds produce no ResultPath, so the
	// Size line is meaningless (and the default arm's "Size: 0 B"
	// headline is actively misleading). All six kinds populate
	// j.Message via p.Set(100, "...") inside the worker; the
	// summary card surfaces that message as the headline so the
	// user sees what the job actually did.
	//
	// Issue #552: Google Drive / Google Sheets uploads now also
	// populate JobResult.RemoteURL + RemoteKind so the summary
	// card can render an "Open in Drive" / "Open in Sheets"
	// button that takes the user to the uploaded artifact.
	// RemoteURL is empty for legacy log entries (those predate
	// the fix and the worker discarded the upload result), so
	// the button is conditionally rendered — see
	// jobs.templ::jobSummaryCard.
	case "image_orphan_cleanup", "duplicate_audit", "review_bulk_resolve", "review_bulk_delete", "google_drive_backup", "google_sheets_export":
		if j.Message != "" {
			s.Headline = j.Message
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		} else {
			// Defensive fallback for an old JSONL log entry that
			// somehow lost its progress message — keep the card
			// usable rather than rendering an empty headline.
			s.Headline = fmt.Sprintf("%s complete.", j.DisplayLabel())
			s.DetailLines = []string{fmt.Sprintf("Duration: %s", formatDuration(s.Duration))}
		}
		if j.Result.RemoteURL != "" {
			label := "Open in Drive"
			switch j.Result.RemoteKind {
			case "sheets":
				label = "Open in Sheets"
			}
			s.RemoteURL = j.Result.RemoteURL
			s.RemoteLabel = label
		}
	default:
		s.Headline = fmt.Sprintf("%s complete — %s.", j.DisplayLabel(), formatBytes(s.SizeBytes))
		s.DetailLines = []string{
			fmt.Sprintf("Size: %s", formatBytes(s.SizeBytes)),
			fmt.Sprintf("Duration: %s", formatDuration(s.Duration)),
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

// appendStaticArchiveStats renders the per-kind content panel
// for the static_archive job (issue #492). Mirrors the GUI
// "Archive contents" panel: a category breakdown of what the
// .zip actually contains, captured at export time.
//
// Ordering: Person Records (with subtype split) first because
// that's the user's primary mental model; events + articles
// follow because they're the other entity types in the
// archive; images + source records + tags round out the
// archive's contents. The Size and Duration lines are
// already on the card before this helper runs.
func appendStaticArchiveStats(lines []string, sa StaticArchiveResult) []string {
	if sa.PersonRecords > 0 {
		lines = append(lines, fmt.Sprintf("Person records: %d", sa.PersonRecords))
	}
	if sa.SpouseRecords > 0 {
		lines = append(lines, fmt.Sprintf("  Spouse records: %d", sa.SpouseRecords))
	}
	if sa.LinkedPeople > 0 {
		lines = append(lines, fmt.Sprintf("  Linked people: %d", sa.LinkedPeople))
	}
	if sa.Events > 0 {
		lines = append(lines, fmt.Sprintf("Events: %d", sa.Events))
	}
	if sa.Articles > 0 {
		lines = append(lines, fmt.Sprintf("Articles: %d", sa.Articles))
	}
	if sa.PersonImages > 0 {
		lines = append(lines, fmt.Sprintf("Person record images: %d", sa.PersonImages))
	}
	if sa.SourceRecords > 0 {
		lines = append(lines, fmt.Sprintf("Source records: %d", sa.SourceRecords))
	}
	if sa.DistinctTags > 0 {
		lines = append(lines, fmt.Sprintf("Distinct tags: %d", sa.DistinctTags))
	}
	// Issue #498 slice 5: Calendar landing + Insights page counts.
	// CalendarDaysWithData is the count of days (across all 12
	// months) that carry at least one anniversary / event /
	// holiday marker — drives the Calendar landing page content.
	// InsightsSections is the count of Insights dimensions with
	// non-empty data — always 7 (record_types + 6 dimensions)
	// even on an empty archive because the record_types dimension
	// is always populated with the headline counts.
	if sa.CalendarDaysWithData > 0 {
		lines = append(lines, fmt.Sprintf("Calendar days with data: %d", sa.CalendarDaysWithData))
	}
	if sa.InsightsSections > 0 {
		lines = append(lines, fmt.Sprintf("Insights sections: %d", sa.InsightsSections))
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

// formatDuration renders a duration as a short, human-friendly
// string for the "Duration:" detail line on the job summary
// card. Format rules (issue #543):
//
//   - elapsed < 60s   ->  "0.8s", "12.4s"   (one decimal place)
//   - elapsed < 60m   ->  "75s"             (whole seconds)
//   - elapsed >= 60m  ->  "1m5s", "12m40s"  (m + remainder seconds)
//
// The sub-second rule restores visibility for fast jobs
// (cleanup / audit / review-bulk / google uploads) whose
// sub-second elapsed time was previously collapsed to "0s"
// by the Round(time.Second) call on s.Duration.
func formatDuration(d time.Duration) string {
	switch {
	case d < 0:
		return "0.0s"
	case d < 60*time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < 60*time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	default:
		mins := int(d / time.Minute)
		secs := int((d % time.Minute) / time.Second)
		return fmt.Sprintf("%dm%ds", mins, secs)
	}
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
		// Read Status + Kind under j.mu so the read is
		// race-free against the worker's writes in
		// Start.func1 (which holds j.mu around every Status
		// mutation at jobs.go:557, :573, :574). r.mu alone
		// is insufficient because the worker writes Status
		// under j.mu only. The subsequent j.Snapshot() call
		// takes j.mu again to copy the rest of the public
		// fields race-free. issue #418.
		j.mu.Lock()
		status := j.Status
		kind := j.Kind
		j.mu.Unlock()
		if status != StatusQueued && status != StatusRunning {
			continue
		}
		if IsSilentKind(kind) {
			continue
		}
		snap := j.Snapshot()
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
//
// Safe to call multiple times: the second-and-later callers attach to
// the same WaitGroup Wait via sync.Once, so test cleanup patterns
// (one Shutdown in the test body + one in t.Cleanup) cannot race the
// WaitGroup reuse check that the sync runtime enforces.
func (r *Registry) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	for _, j := range r.jobs {
		// Read j.Status under j.mu so the read is race-free
		// against the worker's writes in Start.func1 (which
		// holds j.mu around every Status mutation at
		// jobs.go:557, :573, :574). r.mu alone is insufficient
		// because the worker writes Status under j.mu only.
		// Cancel is the per-job context.CancelFunc, captured
		// under the lock so the worker can't swap it out
		// between the read and the call (it doesn't, but
		// the lock + capture pattern is the same shape as
		// the registry uses for the in-flight Set on the
		// job). Cancel is idempotent per the context
		// package contract, so calling it after the worker
		// has already exited is a no-op. issue #418.
		j.mu.Lock()
		status := j.Status
		cancel := j.cancelCause
		j.mu.Unlock()
		if status == StatusQueued || status == StatusRunning {
			cancel()
		}
	}
	r.mu.Unlock()
	var done chan struct{}
	r.shutdownOnce.Do(func() {
		done = make(chan struct{})
		go func() {
			r.workerWG.Wait()
			close(done)
		}()
	})
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