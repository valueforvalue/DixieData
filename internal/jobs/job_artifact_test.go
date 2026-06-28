package jobs

import (
	"testing"
)

// TestIsViewableArtifact is the regression test for issue #129. The
// status page picks between target="_blank" (viewable: open in a new
// tab) and the download attribute (non-viewable: trigger a save
// dialog in the current tab) based on this classification. PDFs and
// JPGs must remain viewable so the existing inline-render fix from
// commit 2f4d587 keeps working. Everything else (ddbak, ddshare, zip,
// csv, ics, json, txt) must report non-viewable so the user gets the
// download attribute instead of a blank tab.
func TestIsViewableArtifact(t *testing.T) {
	cases := []struct {
		path string
		want bool
		desc string
	}{
		{"/tmp/june-2026.pdf", true, "PDF stays viewable (commit 2f4d587)"},
		{"/tmp/photo.jpg", true, "JPEG stays viewable"},
		{"/tmp/photo.jpeg", true, "JPEG (alt ext) stays viewable"},
		{"/tmp/photo.png", true, "PNG viewable"},
		{"/tmp/back-up.ddbak", false, "backup archive must NOT open in new tab (issue #129)"},
		{"/tmp/share.ddshare", false, "shared archive must download in current tab"},
		{"/tmp/archive.zip", false, "static archive zip must download in current tab"},
		{"/tmp/export.csv", false, "CSV must download in current tab"},
		{"/tmp/anniversaries.ics", false, "iCal must download in current tab"},
		{"/tmp/export.json", true, "JSON stays viewable for developer inspection"},
		{"/tmp/missing", false, "no extension defaults to download"},
		{"", false, "empty path defaults to download"},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			j := Job{ResultPath: c.path}
			if got := j.IsViewableArtifact(); got != c.want {
				t.Fatalf("IsViewableArtifact(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

// TestArtifactFilename returns just the base name of ResultPath so
// the status page can use it as the HTML `download` attribute value.
// Without this, browsers default to saving the file as "artifact"
// (the last URL segment), which loses the original filename.
func TestArtifactFilename(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/tmp/june-2026.ddbak", "june-2026.ddbak"},
		{"/var/folders/abc/report.pdf", "report.pdf"},
		{"/tmp/share.ddshare", "share.ddshare"},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			j := Job{ResultPath: c.path}
			if got := j.ArtifactFilename(); got != c.want {
				t.Fatalf("ArtifactFilename(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}