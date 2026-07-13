package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSummaryIsKindAware pins down the per-kind headline copy
// rendered in the redesigned /jobs/{id} summary card (issue
// #131). The exact wording is user-facing so a refactor that
// collapses kinds together or drops the size + duration lines
// immediately fails the regression net.
func TestSummaryIsKindAware(t *testing.T) {
	cases := []struct {
		kind          string
		mustContain   []string
		mustNotContain []string
	}{
		{
			kind:        "static_archive",
			mustContain: []string{"Static archive complete", "Size:", "Duration:"},
		},
		{
			kind:        "database_pdf",
			mustContain: []string{"Printable archive PDF complete", "Size:", "Duration:"},
		},
		{
			kind:        "backup_archive",
			mustContain: []string{"Backup archive complete", "Size:", "Duration:", "Load Backup"},
		},
		{
			kind:        "shared_archive",
			mustContain: []string{"Shared archive complete", "Size:", "Duration:", ".ddshare"},
		},
		{
			kind:        "soldier_pdf",
			mustContain: []string{"complete", "Size:", "Duration:"},
		},
		{
			kind:        "soldier_jpg",
			mustContain: []string{"Soldier JPG export complete", "Size:", "Duration:"},
		},
		{
			kind:        "monthly_pdf",
			mustContain: []string{"Monthly calendar PDF complete"},
		},
		{
			kind:        "backup_import",
			mustContain: []string{"backup_import complete", "Duration:"},
			// Imports don't produce an on-disk artifact the user
			// would download later, so no Size: line is expected.
			mustNotContain: []string{"Size:"},
		},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			dir := t.TempDir()
			resultPath := filepath.Join(dir, "blob.bin")
			if err := os.WriteFile(resultPath, []byte("test bytes"), 0o644); err != nil {
				t.Fatalf("seed artifact: %v", err)
			}
			j := NewJob("job-"+c.kind, c.kind)
			j.Status = StatusDone
			j.StartedAt = time.Now().Add(-2 * time.Second)
			j.FinishedAt = time.Now()
			if c.kind != "backup_import" {
				j.ResultPath = resultPath
			}
			s := j.Summary()
			for _, needle := range c.mustContain {
				if !strings.Contains(s.Headline+s.joinDetails(), needle) {
					t.Errorf("kind=%s summary missing %q\nheadline: %s\ndetails: %v", c.kind, needle, s.Headline, s.DetailLines)
				}
			}
			for _, needle := range c.mustNotContain {
				if strings.Contains(s.Headline+s.joinDetails(), needle) {
					t.Errorf("kind=%s summary unexpectedly contains %q\nheadline: %s\ndetails: %v", c.kind, needle, s.Headline, s.DetailLines)
				}
			}
		})
	}
}

// TestSummaryRunningJobReturnsZero is the safety-path coverage:
// the summary card must not render headline + detail lines for
// a job that's still running, because the user hasn't waited for
// it yet.
func TestSummaryRunningJobReturnsZero(t *testing.T) {
	j := NewJob("job-running", "static_archive")
	j.Status = StatusRunning
	j.Progress = 42
	s := j.Summary()
	if s.Headline != "" {
		t.Errorf("running job must not produce a headline; got %q", s.Headline)
	}
	if len(s.DetailLines) != 0 {
		t.Errorf("running job must not produce detail lines; got %v", s.DetailLines)
	}
}

// TestSummaryDurationRoundedToSecond ensures the duration line
// reads cleanly ("3s", "1m0s", etc.) instead of "3.000000123s".
func TestSummaryDurationRoundedToSecond(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(resultPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	j := NewJob("job-dur", "static_archive")
	j.Status = StatusDone
	j.StartedAt = time.Now().Add(-3*time.Second - 500*time.Millisecond)
	j.FinishedAt = time.Now()
	j.ResultPath = resultPath
	s := j.Summary()
	durLine := ""
	for _, line := range s.DetailLines {
		if strings.HasPrefix(line, "Duration:") {
			durLine = line
			break
		}
	}
	if durLine == "" {
		t.Fatalf("expected a Duration detail line; got %v", s.DetailLines)
	}
	// The line must NOT contain sub-second fractional digits.
	if strings.Contains(durLine, ".") {
		t.Errorf("Duration line should be rounded to whole seconds; got %q", durLine)
	}
	if !strings.Contains(durLine, "4s") {
		t.Errorf("expected Duration line to read 'Duration: 4s'; got %q", durLine)
	}
}

// TestDismissTargetPathIsKindAware covers issue #131's dismiss
// routing. Each kind has a sensible default landing page when no
// referer is saved.
func TestDismissTargetPathIsKindAware(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{"static_archive", "/share"},
		{"database_pdf", "/share"},
		{"backup_archive", "/share"},
		{"shared_archive", "/share"},
		{"backup_import", "/share"},
		{"shared_import", "/share"},
		{"monthly_pdf", "/calendar"},
		{"soldier_pdf", "/soldiers"},
		{"soldier_jpg", "/soldiers"},
		{"image_import", "/browse"},
		{"insights_pdf", "/insights"},
		{"json_export", "/share"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			j := NewJob("job-x", c.kind)
			if got := j.DismissTargetPath(); got != c.want {
				t.Errorf("DismissTargetPath(%q) = %q, want %q", c.kind, got, c.want)
			}
		})
	}
}

// joinDetails is a tiny helper that concatenates Headline +
// DetailLines for substring assertions in the table-driven test.
func (s JobSummary) joinDetails() string {
	out := s.Headline
	for _, line := range s.DetailLines {
		out += "\n" + line
	}
	return out
}

// TestSummaryRendersExportStatsConditionally pins down the rule
// the user picked: summary cards only surface records / images /
// sources counts when the worker populates them. A worker that
// hasn't been upgraded yet (zero counts) renders the original
// Size + Duration copy unchanged.
//
// The test covers the six kinds the user is upgrading
// (json_export, excel_export, icalendar_export, database_pdf,
// static_archive, backup_archive, shared_archive) and the two
// kinds we deliberately leave alone (insights_pdf, bug_report).
func TestSummaryRendersExportStatsConditionally(t *testing.T) {
	type expect struct {
		recordsLine bool
		// Issue #492: imagesLine is now the literal label
		// the summary card renders for the images count.
		// database_pdf / backup_archive / shared_archive use
		// the legacy "Images: 312" string; static_archive
		// uses the per-kind "Person record images: 312" label
		// (so the line is unambiguous in the panel that also
		// shows Person Records).
		imagesLine  string
		sourcesLine bool
	}
	cases := []struct {
		kind   string
		result JobResult
		expect expect
	}{
		// Workers that fill only Records (JSON, Excel, iCal).
		{kind: "json_export", result: JobResult{Records: 247}, expect: expect{recordsLine: true}},
		{kind: "excel_export", result: JobResult{Records: 247}, expect: expect{recordsLine: true}},
		{kind: "icalendar_export", result: JobResult{Records: 247}, expect: expect{recordsLine: true}},
		// Database PDF adds images (the export prints primary
		// images for each record).
		{kind: "database_pdf", result: JobResult{Records: 247, Images: 312}, expect: expect{recordsLine: true, imagesLine: "Images: 312"}},
		// Issue #492: static archive now uses the per-kind
		// StaticArchiveResult struct instead of the legacy
		// Records/Images/Sources triple. The summary card reads
		// the new fields (PersonRecords, PersonImages) via
		// appendStaticArchiveStats. The "Person record images:"
		// line is the disambiguated label (vs. "Images:" which
		// would be ambiguous against the Person Records rows).
		{kind: "static_archive", result: JobResult{StaticArchive: &StaticArchiveResult{PersonRecords: 247, PersonImages: 312}}, expect: expect{recordsLine: true, imagesLine: "Person record images: 312"}},
		// Backup and shared archive include all three counts.
		{kind: "backup_archive", result: JobResult{Records: 247, Images: 312, Sources: 18}, expect: expect{recordsLine: true, imagesLine: "Images: 312", sourcesLine: true}},
		{kind: "shared_archive", result: JobResult{Records: 247, Images: 312, Sources: 18}, expect: expect{recordsLine: true, imagesLine: "Images: 312", sourcesLine: true}},
		// Subset export from the Share Queue includes the same
		// counts as a full shared archive; issue #245.
		{kind: "shared_archive_subset", result: JobResult{Records: 247, Images: 312, Sources: 18}, expect: expect{recordsLine: true, imagesLine: "Images: 312", sourcesLine: true}},
		// Insights and bug report do not enumerate persons — no stats lines.
		{kind: "insights_pdf"},
		{kind: "bug_report"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			dir := t.TempDir()
			resultPath := filepath.Join(dir, "blob.bin")
			if err := os.WriteFile(resultPath, []byte("test bytes"), 0o644); err != nil {
				t.Fatalf("seed artifact: %v", err)
			}
			j := NewJob("job-"+c.kind, c.kind)
			j.Status = StatusDone
			j.StartedAt = time.Now().Add(-2 * time.Second)
			j.FinishedAt = time.Now()
			j.ResultPath = resultPath
			j.Result = c.result
			s := j.Summary()
			body := s.joinDetails()
			if c.expect.recordsLine {
				if !strings.Contains(body, "Person records: 247") {
					t.Errorf("kind=%s expected 'Person records: 247' line; got details=%v", c.kind, s.DetailLines)
				}
			} else {
				if strings.Contains(body, "Person records:") {
					t.Errorf("kind=%s unexpectedly rendered 'Person records:' line; details=%v", c.kind, s.DetailLines)
				}
			}
			if c.expect.imagesLine != "" {
				if !strings.Contains(body, c.expect.imagesLine) {
					t.Errorf("kind=%s expected %q line; got details=%v", c.kind, c.expect.imagesLine, s.DetailLines)
				}
			} else {
				if strings.Contains(body, "Images: 312") {
					t.Errorf("kind=%s unexpectedly rendered 'Images: 312' line; details=%v", c.kind, s.DetailLines)
				}
				if strings.Contains(body, "Person record images: 312") {
					t.Errorf("kind=%s unexpectedly rendered 'Person record images: 312' line; details=%v", c.kind, s.DetailLines)
				}
			}
			if c.expect.sourcesLine {
				if !strings.Contains(body, "Source records: 18") {
					t.Errorf("kind=%s expected 'Source records: 18' line; got details=%v", c.kind, s.DetailLines)
				}
			}
		})
	}
}

// TestAppendStaticArchiveStats_CalendarDaysWithData (issue #498
// slice 5) asserts the summary card surfaces a "Calendar days with
// data: N" line on the static_archive job when CalendarDaysWithData
// is populated. The line is omitted when the count is 0 (per the
// existing conditional-render rule in appendStaticArchiveStats).
func TestAppendStaticArchiveStats_CalendarDaysWithData(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(resultPath, []byte("test bytes"), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	// Case 1: populated count renders the line.
	j1 := NewJob("static-1", "static_archive")
	j1.Status = StatusDone
	j1.StartedAt = time.Now().Add(-2 * time.Second)
	j1.FinishedAt = time.Now()
	j1.ResultPath = resultPath
	j1.Result = JobResult{StaticArchive: &StaticArchiveResult{PersonRecords: 10, CalendarDaysWithData: 42}}
	s1 := j1.Summary()
	if !strings.Contains(s1.joinDetails(), "Calendar days with data: 42") {
		t.Errorf("expected 'Calendar days with data: 42' line; got details=%v", s1.DetailLines)
	}

	// Case 2: zero count omits the line (no noise on empty archives).
	j2 := NewJob("static-2", "static_archive")
	j2.Status = StatusDone
	j2.StartedAt = time.Now().Add(-2 * time.Second)
	j2.FinishedAt = time.Now()
	j2.ResultPath = resultPath
	j2.Result = JobResult{StaticArchive: &StaticArchiveResult{PersonRecords: 10}}
	s2 := j2.Summary()
	if strings.Contains(s2.joinDetails(), "Calendar days with data") {
		t.Errorf("zero count unexpectedly rendered 'Calendar days with data' line; details=%v", s2.DetailLines)
	}
}

// TestAppendStaticArchiveStats_InsightsSections (issue #498 slice 5)
// asserts the summary card surfaces an "Insights sections: N" line
// on the static_archive job when InsightsSections is populated. The
// line is omitted when the count is 0 (defensive — an empty archive
// currently has InsightsSections=1 from record_types alone, but the
// conditional-render rule still applies).
func TestAppendStaticArchiveStats_InsightsSections(t *testing.T) {
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(resultPath, []byte("test bytes"), 0o644); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	j1 := NewJob("static-3", "static_archive")
	j1.Status = StatusDone
	j1.StartedAt = time.Now().Add(-2 * time.Second)
	j1.FinishedAt = time.Now()
	j1.ResultPath = resultPath
	j1.Result = JobResult{StaticArchive: &StaticArchiveResult{PersonRecords: 10, InsightsSections: 7}}
	s1 := j1.Summary()
	if !strings.Contains(s1.joinDetails(), "Insights sections: 7") {
		t.Errorf("expected 'Insights sections: 7' line; got details=%v", s1.DetailLines)
	}

	j2 := NewJob("static-4", "static_archive")
	j2.Status = StatusDone
	j2.StartedAt = time.Now().Add(-2 * time.Second)
	j2.FinishedAt = time.Now()
	j2.ResultPath = resultPath
	j2.Result = JobResult{StaticArchive: &StaticArchiveResult{PersonRecords: 10}}
	s2 := j2.Summary()
	if strings.Contains(s2.joinDetails(), "Insights sections") {
		t.Errorf("zero count unexpectedly rendered 'Insights sections' line; details=%v", s2.DetailLines)
	}
}

// TestSummaryRendersSharedImportStats pins down the merge-review
// headline (Added/Merged/Skipped) plus the conflicts reminder.
// When Conflicts > 0 the user is told to open Merge Review; when
// 0 the line is absent so a clean import stays clean.
func TestSummaryRendersSharedImportStats(t *testing.T) {
	t.Run("clean import", func(t *testing.T) {
		j := NewJob("job-clean", "shared_import")
		j.Status = StatusDone
		j.StartedAt = time.Now().Add(-2 * time.Second)
		j.FinishedAt = time.Now()
		j.Result = JobResult{Added: 5, Merged: 3, Skipped: 12}
		s := j.Summary()
		body := s.joinDetails()
		if !strings.Contains(body, "5 added, 3 merged, 12 skipped") {
			t.Errorf("expected merge headline; got %v", s.DetailLines)
		}
		if strings.Contains(body, "Conflicts staged for review") {
			t.Errorf("clean import must not show conflicts line; got %v", s.DetailLines)
		}
	})
	t.Run("conflicts present", func(t *testing.T) {
		j := NewJob("job-conf", "shared_import")
		j.Status = StatusDone
		j.StartedAt = time.Now().Add(-2 * time.Second)
		j.FinishedAt = time.Now()
		j.Result = JobResult{Added: 2, Merged: 1, Skipped: 0, Conflicts: 4}
		s := j.Summary()
		body := s.joinDetails()
		if !strings.Contains(body, "2 added, 1 merged, 0 skipped") {
			t.Errorf("expected merge headline; got %v", s.DetailLines)
		}
		if !strings.Contains(body, "Conflicts staged for review: 4") {
			t.Errorf("expected conflicts reminder; got %v", s.DetailLines)
		}
	})
	t.Run("images and sources imported", func(t *testing.T) {
		j := NewJob("job-imp", "shared_import")
		j.Status = StatusDone
		j.StartedAt = time.Now().Add(-2 * time.Second)
		j.FinishedAt = time.Now()
		j.Result = JobResult{Added: 1, Merged: 0, Skipped: 0, ImagesImported: 14, SourcesImported: 6}
		s := j.Summary()
		body := s.joinDetails()
		if !strings.Contains(body, "Images imported: 14") {
			t.Errorf("expected images imported line; got %v", s.DetailLines)
		}
		if !strings.Contains(body, "Source records imported: 6") {
			t.Errorf("expected sources imported line; got %v", s.DetailLines)
		}
	})
}

// TestSummaryRendersBackupRestoreStats pins down the
// replace-semantics summary: replaced counts and schema parity.
// The schema line is always shown when either schema field is
// populated; the wording switches on MigrationRan.
func TestSummaryRendersBackupRestoreStats(t *testing.T) {
	t.Run("schema migrated", func(t *testing.T) {
		j := NewJob("job-mig", "backup_import")
		j.Status = StatusDone
		j.StartedAt = time.Now().Add(-2 * time.Second)
		j.FinishedAt = time.Now()
		j.Result = JobResult{ReplacedRecords: 247, ReplacedImages: 312, BackupSchema: 5, CurrentSchema: 7, MigrationRan: true}
		s := j.Summary()
		body := s.joinDetails()
		if !strings.Contains(body, "Replaced: 247 records, 312 images") {
			t.Errorf("expected replaced line; got %v", s.DetailLines)
		}
		if !strings.Contains(body, "Schema migrated: backup v5 → current v7") {
			t.Errorf("expected migration line; got %v", s.DetailLines)
		}
	})
	t.Run("schema equal", func(t *testing.T) {
		j := NewJob("job-eq", "backup_import")
		j.Status = StatusDone
		j.StartedAt = time.Now().Add(-2 * time.Second)
		j.FinishedAt = time.Now()
		j.Result = JobResult{ReplacedRecords: 247, ReplacedImages: 312, BackupSchema: 7, CurrentSchema: 7}
		s := j.Summary()
		body := s.joinDetails()
		if !strings.Contains(body, "Schema: backup v7 = current v7 (no migration)") {
			t.Errorf("expected schema-equality line; got %v", s.DetailLines)
		}
	})
}

// TestSummaryRendersMemorialImportStats pins down the
// memorial-import headline (Added/Skipped/Failed) and the
// optional images line.
func TestSummaryRendersMemorialImportStats(t *testing.T) {
	j := NewJob("job-mem", "memorial_import")
	j.Status = StatusDone
	j.StartedAt = time.Now().Add(-2 * time.Second)
	j.FinishedAt = time.Now()
	j.Result = JobResult{Added: 18, Skipped: 3, Failed: 2, ImagesImported: 4}
	s := j.Summary()
	body := s.joinDetails()
	if !strings.Contains(body, "18 added, 3 skipped, 2 failed") {
		t.Errorf("expected memorial headline; got %v", s.DetailLines)
	}
	if !strings.Contains(body, "Images imported: 4") {
		t.Errorf("expected images imported line; got %v", s.DetailLines)
	}
}

// TestSummaryZeroStateKindsAreMessageDriven pins down the fix for
// issue #543: six kinds produce no ResultPath and therefore
// cannot render the default "Size: 0 B / Duration: 0s" card.
// The workers (settings / reviews / insights / google handlers)
// populate j.Message via p.Set(100, "...") before the job
// transitions to StatusDone, so the summary card must:
//
//   - anchor the headline on j.Message so the user sees what
//     the worker actually did ("Moved 3 image(s) into temp trash.")
//   - skip the Size: line (no on-disk artifact to size)
//   - format the duration with sub-second precision so a 800ms
//     cleanup does not collapse to "Duration: 0s" (the bug-2
//     symptom called out in the issue triage)
//
// These tests were red on the pre-fix code: the kinds fell through
// to `default`, which formatted Size/Duration even when the
// ResultPath was empty.
func TestSummaryZeroStateKindsAreMessageDriven(t *testing.T) {
	cases := []struct {
		kind    string
		message string
	}{
		{kind: "image_orphan_cleanup", message: "Moved 3 image(s) into temp trash."},
		{kind: "duplicate_audit", message: "Scanned 247 records, 12 candidate pairs (4 suppressed)."},
		{kind: "review_bulk_resolve", message: "Resolved 8 review queue item(s)."},
		{kind: "review_bulk_delete", message: "Deleted 5 review queue record(s)."},
		{kind: "google_drive_backup", message: "Uploaded 247 soldiers, 1240 images."},
		{kind: "google_sheets_export", message: "Google Sheet ready."},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			j := NewJob("job-"+c.kind, c.kind)
			j.Status = StatusDone
			j.Message = c.message
			// 800ms elapsed — well below the old 1-second round
			// boundary that collapsed this to "Duration: 0s".
			j.StartedAt = time.Now().Add(-800 * time.Millisecond)
			j.FinishedAt = time.Now()
			// Deliberately no ResultPath: these kinds do not
			// write an artifact the user can download later.
			s := j.Summary()
			body := s.joinDetails()
			// 1. headline uses the worker's progress message.
			if !strings.Contains(body, c.message) {
				t.Errorf("kind=%s summary must contain worker Message %q; got headline=%q details=%v",
					c.kind, c.message, s.Headline, s.DetailLines)
			}
			// 2. no Size line — no artifact to size.
			if strings.Contains(body, "Size:") {
				t.Errorf("kind=%s summary must not contain 'Size:' line for no-artifact kind; got details=%v",
					c.kind, s.DetailLines)
			}
			// 3. duration is sub-second-friendly (0.8s, not 0s).
			if strings.Contains(body, "Duration: 0s") {
				t.Errorf("kind=%s sub-second duration collapsed to 'Duration: 0s'; got details=%v",
					c.kind, s.DetailLines)
			}
			if !strings.Contains(body, "0.8s") {
				t.Errorf("kind=%s expected sub-second duration '0.8s'; got details=%v",
					c.kind, s.DetailLines)
			}
		})
	}
}