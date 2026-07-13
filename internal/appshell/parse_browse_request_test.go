// Issue #520 regression net: parseBrowseRequest must preserve
// multi-value semantics for the tags form key. The Browse filter
// UI emits one <input type="checkbox" name="tags"> per pill, so
// selecting two pills produces url.Values{"tags": ["a", "b"]}.
// The previous implementation called values.Get("tags") which
// returns only the first value — silently dropping every tag
// past the first and making the AND-logic SQL unreachable.
//
// This test pins the round-trip for both shapes the Browse
// filter produces:
//   - Multi-checkbox form submit: url.Values{"tags": ["a", "b"]}
//     → BrowseRequest{Tags: ["a", "b"]}
//   - URL-deep-link form: ?tags=foo,bar → ["foo", "bar"]
//   - Mixed: a checkbox submit that also carries the deep-link
//     form key, or vice versa — both entries survive.
//
// The Browse SQL test in internal/records/browse_test.go owns
// the AND-logic contract (HAVING COUNT(DISTINCT t.id) = N). This
// test only owns the handler-level predicate that hands the SQL
// its tag slice.
package appshell

import (
	"net/url"
	"reflect"
	"testing"
)

func TestParseBrowseRequestTagsPreservesMultiValueSemantics(t *testing.T) {
	cases := []struct {
		name string
		in   url.Values
		want []string
	}{
		{
			name: "single-checkbox submit round-trips",
			in:   url.Values{"tags": {"wounded"}},
			want: []string{"wounded"},
		},
		{
			name: "two-checkbox submit preserves both entries",
			in:   url.Values{"tags": {"wounded", "pow"}},
			want: []string{"wounded", "pow"},
		},
		{
			name: "three-checkbox submit preserves all three entries",
			in:   url.Values{"tags": {"wounded", "pow", "kia"}},
			want: []string{"wounded", "pow", "kia"},
		},
		{
			name: "URL-deep-link form splits on commas",
			in:   url.Values{"tags": {"wounded,pow"}},
			want: []string{"wounded", "pow"},
		},
		{
			name: "deep-link form with whitespace + empties",
			in:   url.Values{"tags": {" wounded , , pow "}},
			want: []string{"wounded", "pow"},
		},
		{
			name: "no tags key returns nil slice",
			in:   url.Values{},
			want: []string{},
		},
		{
			name: "empty tags value returns nil slice",
			in:   url.Values{"tags": {""}},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBrowseRequest(tc.in).Tags
			// Empty-slice and nil-slice are interchangeable for our
			// purposes; reflect.DeepEqual distinguishes them, so
			// normalize the want to the actual got shape.
			want := tc.want
			if want == nil {
				want = []string{}
			}
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Tags = %#v, want %#v", got, want)
			}
		})
	}
}
