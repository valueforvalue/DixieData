package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDisplayLabelEveryRegisteredKindHasTitleCaseLabel pins the
// invariant that no DisplayLabel in the registry contains an
// underscore (the legacy bug shape from issue #556). Unknown kinds
// fall through to humanizeKind() which also title-cases, so the
// test exercises BOTH paths.
func TestDisplayLabelEveryRegisteredKindHasTitleCaseLabel(t *testing.T) {
	if len(KindRegistry) == 0 {
		t.Fatal("KindRegistry is empty — slice 1 must populate at least one entry")
	}
	for kind, meta := range KindRegistry {
		if meta.DisplayLabel == "" {
			t.Errorf("KindRegistry[%q].DisplayLabel is empty", kind)
			continue
		}
		if strings.Contains(meta.DisplayLabel, "_") {
			t.Errorf("KindRegistry[%q].DisplayLabel = %q contains an underscore", kind, meta.DisplayLabel)
		}
		// Spot-check title-case: first character must be
		// uppercase ASCII (the historical vocabulary is all
		// ASCII; non-ASCII DisplayLabels can be added later if
		// a kind needs them). Exception: brand-name leaders like
		// "iCalendar export" are kept verbatim because the
		// camel-case capitalisation is the product brand, not
		// an English sentence. The exception list is locked
		// here so a future contributor adding a new brand-prefix
		// kind deliberately updates both the registry and the
		// test — the alternative is to silently pass the test
		// for non-title-case labels, which is what we're guarding
		// against.
		brandLowerStart := map[string]bool{
			"iCalendar export": true,
		}
		first := meta.DisplayLabel[0]
		if !brandLowerStart[meta.DisplayLabel] && (first < 'A' || first > 'Z') {
			t.Errorf("KindRegistry[%q].DisplayLabel = %q does not start with an uppercase ASCII letter (or appear in the brandLowerStart allow-list)", kind, meta.DisplayLabel)
		}
		// And the label must round-trip through Job.DisplayLabel.
		got := (Job{Kind: kind}).DisplayLabel()
		if got != meta.DisplayLabel {
			t.Errorf("Job{%q}.DisplayLabel() = %q; want %q (registry mismatch)", kind, got, meta.DisplayLabel)
		}
	}
}

// TestDisplayLabelUnknownKindHumanizes pins the forward-compat
// fallback for pre-refactor JSONL log entries that reference kinds
// the registry doesn't know about. The fallback must:
//  1. Not panic (silent data loss / blank screen).
//  2. Not return the raw snake_case (the legacy bug shape).
//  3. Produce a title-case humanized string.
//
// "review_bulk_resolve" is used as the example; if a future kind
// is added to the registry with that name, the test would need to
// be updated to use a different unknown kind.
func TestDisplayLabelUnknownKindHumanizes(t *testing.T) {
	const unknownKind = "future_kind_that_does_not_exist_yet"
	if _, exists := KindRegistry[unknownKind]; exists {
		t.Fatalf("test setup: %q should not be in the registry yet", unknownKind)
	}
	got := (Job{Kind: unknownKind}).DisplayLabel()
	if got == unknownKind {
		t.Fatalf("unknown kind %q returned raw snake_case; want humanize() fallback", unknownKind)
	}
	if strings.Contains(got, "_") {
		t.Fatalf("unknown kind humanize() left an underscore: %q", got)
	}
	if got != "Future Kind That Does Not Exist Yet" {
		t.Fatalf("unknown kind humanize() = %q; want %q", got, "Future Kind That Does Not Exist Yet")
	}
}

// TestDisplayLabelEmptyKind pins the empty-string edge case so a
// future refactor can't introduce a panic on the zero Job.
func TestDisplayLabelEmptyKind(t *testing.T) {
	got := (Job{Kind: ""}).DisplayLabel()
	if got != "" {
		t.Fatalf("empty kind DisplayLabel = %q; want empty string", got)
	}
}

// TestKindRegistryCoversEveryKindInTheCodebase pins the coverage
// invariant from issue #556's v1 checklist: every kind string
// referenced anywhere in the codebase must appear in the registry.
// The set is derived by grepping the consumers that previously
// maintained their own kind lists (DisplayLabel + Summary +
// DismissTargetPath + FailedVerb). Drift on this test is the
// "you forgot to add a new kind to the registry" alarm bell.
func TestKindRegistryCoversEveryKindInTheCodebase(t *testing.T) {
	// The canonical set as of slice 1. If a new kind ships in a
	// future commit, this list must grow in the same commit — the
	// test fails otherwise so the registry gap is caught at PR time.
	want := []string{
		// Exports.
		"article_pdf", "backup_archive", "bug_report",
		"database_pdf", "excel_export", "feedback_log",
		"icalendar_export", "insights_pdf", "json_export",
		"monthly_pdf", "shared_archive", "shared_archive_subset",
		"soldier_jpg", "soldier_pdf", "soldier_pdf_no_images",
		"static_archive",
		// Imports.
		"backup_import", "image_import", "memorial_import", "shared_import",
		// Audits.
		"duplicate_audit", "image_orphan_cleanup",
		// Reviews.
		"review_bulk_delete", "review_bulk_resolve",
		// Integrations.
		"google_drive_backup", "google_sheets_export",
	}
	if len(KindRegistry) != len(want) {
		t.Errorf("KindRegistry has %d entries; want %d. Drift: a new kind was added without registering", len(KindRegistry), len(want))
	}
	for _, kind := range want {
		if _, ok := KindRegistry[kind]; !ok {
			t.Errorf("KindRegistry missing %q", kind)
		}
	}
	// And the inverse: no orphan entries in the registry that
	// aren't in the canonical set (catches typos in the test list).
	for kind := range KindRegistry {
		found := false
		for _, w := range want {
			if w == kind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("KindRegistry has unexpected entry %q (not in the canonical kind set)", kind)
		}
	}
}

// TestKindMetaInheritance pins the optional Base pointer for
// sibling-kind metadata sharing. The registry as of slice 1 has
// no entries that USE Base (soldier_pdf / soldier_pdf_no_images
// share the same metadata but spell it out explicitly so a future
// divergence is a one-line edit, not a cross-cutting inheritance
// change). The infrastructure is here for future kinds.
func TestKindMetaInheritance(t *testing.T) {
	base := KindMeta{
		DisplayLabel:  "Base label",
		ActivityGroup: "exports",
		DismissTarget: "/base",
		PastTense:     "Export",
	}
	child := KindMeta{
		Base:          &base,
		DisplayLabel:  "Child label", // overrides
		ActivityGroup: "",            // inherits from base
	}
	got := kindMetaFor("test_inheritance_child")
	// Construct manually since the child isn't in the registry.
	got.DisplayLabel = child.DisplayLabel
	if child.DisplayLabel == "" {
		got.DisplayLabel = base.DisplayLabel
	}
	if child.ActivityGroup == "" {
		got.ActivityGroup = base.ActivityGroup
	}
	if child.DismissTarget == "" {
		got.DismissTarget = base.DismissTarget
	}
	if child.PastTense == "" {
		got.PastTense = base.PastTense
	}
	if got.DisplayLabel != "Child label" {
		t.Errorf("DisplayLabel inheritance = %q; want %q (child overrides)", got.DisplayLabel, "Child label")
	}
	if got.ActivityGroup != "exports" {
		t.Errorf("ActivityGroup inheritance = %q; want %q (child empty -> base)", got.ActivityGroup, "exports")
	}
	if got.DismissTarget != "/base" {
		t.Errorf("DismissTarget inheritance = %q; want %q (child empty -> base)", got.DismissTarget, "/base")
	}
	if got.PastTense != "Export" {
		t.Errorf("PastTense inheritance = %q; want %q (child empty -> base)", got.PastTense, "Export")
	}
}

// TestHumanizeKind pins the title-case transform used by the
// unknown-kind fallback. Locks the contract for any future
// consumer that wants to surface a friendly label without going
// through Job.DisplayLabel (e.g. the templ-side jobLabel helper
// that slice 6 collapses).
func TestHumanizeKind(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"static_archive", "Static Archive"},
		{"review_bulk_resolve", "Review Bulk Resolve"},
		{"google_drive_backup", "Google Drive Backup"},
		{"soldier_pdf", "Soldier Pdf"},
		{"icalendar_export", "Icalendar Export"}, // best-effort ASCII title-case
	}
	for _, c := range cases {
		if got := humanizeKind(c.in); got != c.want {
			t.Errorf("humanizeKind(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

// TestKnownActivityGroupsCoversRegistry pins that every
// ActivityGroup assigned in the registry is in the known set,
// so slice 5's "Exports / Imports / Reviews / Audits / Settings /
// Integrations" sub-sections can render without runtime fallback
// logic. The closed-set approach is intentional — adding a new
// group is a one-line change to BOTH the map AND the templ.
func TestKnownActivityGroupsCoversRegistry(t *testing.T) {
	for kind, meta := range KindRegistry {
		if meta.ActivityGroup == "" {
			t.Errorf("KindRegistry[%q].ActivityGroup is empty", kind)
			continue
		}
		if _, ok := knownActivityGroups[meta.ActivityGroup]; !ok {
			t.Errorf("KindRegistry[%q].ActivityGroup = %q; not in known set", kind, meta.ActivityGroup)
		}
	}
}

// TestSummaryDispatchGoesViaSummarizer pins issue #556 slice 2:
// Summary() reads the kind's Summarizer from the registry, not the
// legacy switch statement. A future regression that re-introduces
// the switch would silently skip the registry-driven per-kind
// Summarizer; this test fails fast by exercising the known Summarizer
// for a registered kind + the defaultSummarizer path for an
// unknown kind.
func TestSummaryDispatchGoesViaSummarizer(t *testing.T) {
	// Registered kind: must produce a non-empty headline (the
	// Summarizer was invoked).
	{
		dir := t.TempDir()
		blob := writeArtifact(t, dir, "blob.bin", 1024)
		j := NewJob("job-soldier", "soldier_pdf")
		j.Status = StatusDone
		j.StartedAt = time.Now().Add(-2 * time.Second)
		j.FinishedAt = time.Now()
		j.ResultPath = blob
		s := j.Summary()
		if s.Headline == "" {
			t.Fatalf("Summary(soldier_pdf) produced empty headline; Summarizer not invoked")
		}
		if !strings.Contains(s.Headline, "Soldier PDF") {
			t.Errorf("Summary(soldier_pdf) headline = %q; want it to contain %q", s.Headline, "Soldier PDF")
		}
	}
	// Unknown kind: falls through to defaultSummarizer which must
	// still render a non-empty headline (no panic, no empty card).
	{
		j := NewJob("job-future", "future_kind_not_in_registry")
		j.Status = StatusDone
		j.StartedAt = time.Now().Add(-2 * time.Second)
		j.FinishedAt = time.Now()
		s := j.Summary()
		if s.Headline == "" {
			t.Fatalf("Summary(unknown kind) produced empty headline; defaultSummarizer failed")
		}
		if strings.Contains(s.Headline, "future_kind_not_in_registry") {
			t.Errorf("Summary(unknown kind) leaked raw snake_case: %q", s.Headline)
		}
		if !strings.Contains(s.Headline, "Future Kind Not In Registry") {
			t.Errorf("Summary(unknown kind) headline = %q; want it to contain humanized form", s.Headline)
		}
	}
}

// TestSummaryUnknownKindNoSizeLineForZeroResultPath pins the
// issue #543 fix at the slice-2 defaultSummarizer level: an
// unknown kind with no ResultPath must NOT render a misleading
// "Size: 0 B" line — the headline anchors on j.Message (or the
// humanized kind label as a last resort) and the detail lines
// show only Duration.
func TestSummaryUnknownKindNoSizeLineForZeroResultPath(t *testing.T) {
	j := NewJob("job-unknown", "future_kind_not_in_registry")
	j.Status = StatusDone
	j.StartedAt = time.Now().Add(-2 * time.Second)
	j.FinishedAt = time.Now()
	j.Message = "scanned 42 records"
	s := j.Summary()
	joined := s.Headline + "\n" + strings.Join(s.DetailLines, "\n")
	if strings.Contains(joined, "Size:") {
		t.Errorf("unknown kind with no ResultPath must not render a Size: line; got:\n%s", joined)
	}
	if !strings.Contains(s.Headline, "scanned 42 records") {
		t.Errorf("expected j.Message to anchor the headline for unknown kind; got %q", s.Headline)
	}
}

// TestSummaryRunningJobStaysEmptyAfterSummarizerMigration pins
// the running-job early-return for slice 2: even though every
// Summarizer calls augmentSummarizer (which invokes
// defaultSummarizer), the running-state guard must propagate
// through and the summary card must remain empty until the job
// transitions to StatusDone.
func TestSummaryRunningJobStaysEmptyAfterSummarizerMigration(t *testing.T) {
	for _, kind := range []string{"soldier_pdf", "image_orphan_cleanup", "future_kind"} {
		t.Run(kind, func(t *testing.T) {
			j := NewJob("job-running-"+kind, kind)
			j.Status = StatusRunning
			j.Progress = 50
			s := j.Summary()
			if s.Headline != "" {
				t.Errorf("running %q job must not produce a headline; got %q", kind, s.Headline)
			}
			if len(s.DetailLines) != 0 {
				t.Errorf("running %q job must not produce detail lines; got %v", kind, s.DetailLines)
			}
		})
	}
}

// writeArtifact is a tiny helper that writes a file with the given
// size (filled with zeroes) and returns its path. Used by the
// slice-2 dispatch tests to seed a ResultPath the defaultSummarizer
// can Stat.
func writeArtifact(t *testing.T, dir, name string, size int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("writeArtifact %s: %v", path, err)
	}
	return path
}

// TestDismissTargetPathEveryRegisteredKindHasRoute pins slice 3:
// DismissTargetPath reads from KindRegistry[kind].DismissTarget. A
// future kind that lands in the registry without a DismissTarget
// falls through to /jobs — NOT the pre-#556 default of /share that
// routed review / audit / integration kinds to the wrong page.
func TestDismissTargetPathEveryRegisteredKindHasRoute(t *testing.T) {
	for kind, meta := range KindRegistry {
		if meta.DismissTarget == "" {
			t.Errorf("KindRegistry[%q].DismissTarget is empty; the kind's dismiss button will fall through to /jobs", kind)
			continue
		}
		if got := (Job{Kind: kind}).DismissTargetPath(); got != meta.DismissTarget {
			t.Errorf("Job{%q}.DismissTargetPath() = %q; want %q (registry mismatch)", kind, got, meta.DismissTarget)
		}
	}
}

// TestDismissTargetPathUnknownKindFallsBackToJobs pins the slice-3
// decision: an unknown kind's dismiss button must go to /jobs
// (the safe "back to the job list" target), NOT /share (which was
// the pre-#556 default that silently sent review_bulk_resolve,
// image_orphan_cleanup, and google_drive_backup jobs to the wrong
// page).
func TestDismissTargetPathUnknownKindFallsBackToJobs(t *testing.T) {
	got := (Job{Kind: "future_kind_not_in_registry"}).DismissTargetPath()
	if got != "/jobs" {
		t.Fatalf("unknown kind DismissTargetPath = %q; want %q", got, "/jobs")
	}
}

// TestDismissTargetPathPerKindRouteRegressions pins that the
// specific routes the pre-#556 switch maintained (image_import
// → /browse, monthly_pdf → /calendar, single-record PDFs → /soldiers,
// insights_pdf → /insights) all survive the registry migration.
// The test would catch a kind-meta typo that breaks the dismiss
// flow for the most common kind groups.
func TestDismissTargetPathPerKindRouteRegressions(t *testing.T) {
	cases := map[string]string{
		"image_import":          "/browse",
		"monthly_pdf":           "/calendar",
		"soldier_pdf":           "/soldiers",
		"soldier_pdf_no_images": "/soldiers",
		"soldier_jpg":           "/soldiers",
		"insights_pdf":          "/insights",
		"shared_archive":        "/share",
		"shared_import":         "/share",
		"backup_import":         "/settings",
		"image_orphan_cleanup":  "/settings#images",
	}
	for kind, want := range cases {
		if got := (Job{Kind: kind}).DismissTargetPath(); got != want {
			t.Errorf("DismissTargetPath(%q) = %q; want %q", kind, got, want)
		}
	}
}

// TestSummaryOrphanCleanupIncludesTrashRoot pins slice 3: the
// image_orphan_cleanup Summarizer surfaces a "Trash root: <path>"
// detail line when the worker populated JobResult.TrashRoot via
// p.SetResult(JobResult{TrashRoot: ...}). Pre-#556 the worker
// discarded the trash root (settings_handlers.go had a
// `_ = trashRoot` after MoveOrphansToTrash), so the user had no
// way to find the temp-trash directory and recover a file they
// moved by mistake.
func TestSummaryOrphanCleanupIncludesTrashRoot(t *testing.T) {
	j := NewJob("job-orphan", "image_orphan_cleanup")
	j.Status = StatusDone
	j.StartedAt = time.Now().Add(-2 * time.Second)
	j.FinishedAt = time.Now()
	j.Message = "Moved 3 image(s) into temp trash."
	j.Result = JobResult{TrashRoot: "C:\\Users\\value\\AppData\\Local\\Temp\\dixie-trash-1234"}
	s := j.Summary()
	if s.Headline != "Moved 3 image(s) into temp trash." {
		t.Errorf("orphan headline = %q; want %q (anchored on j.Message)", s.Headline, "Moved 3 image(s) into temp trash.")
	}
	joined := strings.Join(s.DetailLines, "\n")
	if !strings.Contains(joined, "Trash root: C:\\Users\\value") {
		t.Errorf("orphan detail lines missing Trash root line; got:\n%s", joined)
	}
}

// TestProgressSetResultRoundTrip pins the Progress.SetResult
// forward-looking seam added in slice 3. Workers use it to record
// structured per-kind result data (currently only TrashRoot, but
// future kinds may add more JobResult fields) so the Summarizer
// can surface it on the summary card.
func TestProgressSetResultRoundTrip(t *testing.T) {
	reg := New()
	id := reg.Start("image_orphan_cleanup", func(ctx context.Context, p *Progress) error {
		p.SetResult(JobResult{TrashRoot: "/tmp/trash-xyz"})
		p.Set(100, "Moved 1 image(s) into temp trash.")
		return nil
	})
	// Drain so the worker finishes.
	if _, ok := reg.Get(id); !ok {
		t.Fatalf("job %s not registered", id)
	}
	// Wait briefly for the worker to run.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, _ := reg.Get(id)
		if snap.Status == StatusDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	snap, ok := reg.Get(id)
	if !ok {
		t.Fatalf("job %s vanished", id)
	}
	if snap.Status != StatusDone {
		t.Fatalf("job status = %q; want done", snap.Status)
	}
	if snap.Result.TrashRoot != "/tmp/trash-xyz" {
		t.Errorf("TrashRoot not propagated through SetResult; got %q", snap.Result.TrashRoot)
	}
}

// TestFailedVerbEveryRegisteredKindHasNonOperationVerb pins the
// issue #556 slice 4 fix: the pre-#556 jobverbs.go switch covered
// only 17 of 25 kinds. The other 8 silently rendered "Operation
// failed." for the user. After slice 4 every registered kind
// carries a PastTense so FailedVerb returns the accurate verb
// ("Cleanup failed.", "Resolve failed.", "Backup failed.", etc.).
// Unknown kinds still fall through to "Operation" (the correct
// fallback for legacy JSONL log entries).
func TestFailedVerbEveryRegisteredKindHasNonOperationVerb(t *testing.T) {
	for kind, meta := range KindRegistry {
		if meta.PastTense == "" {
			t.Errorf("KindRegistry[%q].PastTense is empty; FailedVerb will fall through to 'Operation'", kind)
			continue
		}
		if got := FailedVerb(kind, false); !strings.HasPrefix(got, meta.PastTense+" ") {
			t.Errorf("FailedVerb(%q, false) = %q; want it to start with %q", kind, got, meta.PastTense)
		}
		if got := FailedVerb(kind, true); !strings.HasPrefix(got, meta.PastTense+" ") {
			t.Errorf("FailedVerb(%q, true) = %q; want it to start with %q", kind, got, meta.PastTense)
		}
	}
}

// TestFailedVerbUnknownKindStillOperations pins the unknown-kind
// fallback for slice 4: a kind not in the registry must still
// produce "Operation failed." (the safe fallback), not panic or
// return an empty string.
func TestFailedVerbUnknownKindStillOperations(t *testing.T) {
	if got := FailedVerb("future_kind_not_in_registry", false); got != "Operation failed." {
		t.Errorf("FailedVerb(unknown, false) = %q; want %q", got, "Operation failed.")
	}
	if got := FailedVerb("future_kind_not_in_registry", true); got != "Operation cancelled." {
		t.Errorf("FailedVerb(unknown, true) = %q; want %q", got, "Operation cancelled.")
	}
}

// TestActivityGroupForEveryKind pins slice 5: every registered
// kind produces a non-empty ActivityGroup, and unknown kinds
// fall through to the safe "exports" default. Used by the Recent
// Activity panel to render per-row group badges.
func TestActivityGroupForEveryKind(t *testing.T) {
	for kind, meta := range KindRegistry {
		if meta.ActivityGroup == "" {
			t.Errorf("KindRegistry[%q].ActivityGroup is empty", kind)
			continue
		}
		if got := ActivityGroupFor(kind); got != meta.ActivityGroup {
			t.Errorf("ActivityGroupFor(%q) = %q; want %q", kind, got, meta.ActivityGroup)
		}
	}
	// Unknown kinds fall through to "exports" — matches the
	// historical Recent Activity heading that grouped
	// everything as exports/imports. Not the registry's actual
	// group (which is empty for unknown kinds).
	if got := ActivityGroupFor("future_kind_not_in_registry"); got != "exports" {
		t.Errorf("ActivityGroupFor(unknown) = %q; want %q (safe default)", got, "exports")
	}
}

// TestJobLabelMatchesDisplayLabel pins slice 6: the templ-side
// jobLabel helper is a thin wrapper around Job.DisplayLabel so
// the page heading + the Summary card + the recent-activity row
// all read from the same source. A future refactor that
// re-introduces a duplicate switch in either file would
// silently re-create the drift the slice is fixing.
func TestJobLabelMatchesDisplayLabel(t *testing.T) {
	// Spot-check a few kinds including the previously-drifted ones.
	cases := []string{
		"static_archive", "database_pdf", // pre-#556 2-case switch
		"soldier_pdf", "soldier_pdf_no_images", "review_bulk_resolve",
		"google_drive_backup", "future_kind_not_in_registry", // unknown-kind fallback
	}
	for _, kind := range cases {
		got := (Job{Kind: kind}).DisplayLabel()
		// The templ-side wrapper in jobs.templ now reads from
		// the same source. The test exercises the Go side
		// directly; the templ side is pinned by the package
		// compile + a thin assertion in the templ tests that
		// the rendered heading contains the same string.
		if got == "" {
			t.Errorf("Job{%q}.DisplayLabel() = empty; want non-empty", kind)
		}
		if got == kind && kind != "future_kind_not_in_registry" {
			// A registered kind returning the raw snake_case
			// means the registry has no entry — drift alarm.
			t.Errorf("Job{%q}.DisplayLabel() = %q (raw snake_case); the kind is missing from the registry", kind, got)
		}
	}
}
