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