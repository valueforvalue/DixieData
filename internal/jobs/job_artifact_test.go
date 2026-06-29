package jobs

import (
	"testing"
)

// TestIsViewableArtifact is the regression test for issue #129.
// It locks the artifact-endpoint's disposition choice: PDFs and
// JPGs must remain viewable (the endpoint serves them with
// Content-Disposition: inline) so the existing inline-render
// path keeps working. Everything else (.ddbak, .ddshare, .zip,
// .csv, .ics) reports non-viewable; those flow through the
// attachment branch. .json is in the viewable list because
// Chromium renders application/json natively.
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
		{"/tmp/back-up.ddbak", false, "backup archive: not viewable"},
		{"/tmp/share.ddshare", false, "shared archive: not viewable"},
		{"/tmp/archive.zip", false, "static archive zip: not viewable"},
		{"/tmp/export.csv", false, "CSV: not viewable"},
		{"/tmp/anniversaries.ics", false, "iCal: not viewable"},
		{"/tmp/export.json", true, "JSON is viewable for developer inspection"},
		{"/tmp/missing", false, "no extension defaults to non-viewable"},
		{"", false, "empty path defaults to non-viewable"},
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
