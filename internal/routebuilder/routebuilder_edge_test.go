package routebuilder

import "testing"

// TestJobStatusEdgeCases pins the two behaviors the one-liner
// string-concat tests used to cover individually: URL-escaping
// the job ID and trimming surrounding whitespace. Every other
// route helper is a plain `fmt`-style concat; the lint-grade
// catches drift, and these two were the only non-trivial cases.
func TestJobStatusEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"escapes slash and space", "id with/slash", "/jobs/id%20with%2Fslash/status"},
		{"trims surrounding whitespace", "  spaced  ", "/jobs/spaced/status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := JobStatus(tc.in); got != tc.want {
				t.Fatalf("JobStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
