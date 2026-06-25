package stress

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/findagrave"
)

func TestFindAGraveMalformedHTMLCases(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test: run via `make stress` or `go test ./tests/stress/...`")
	}
	t.Parallel()

	cases := []string{
		`<html><body><div id="bio-name">Broken`,
		`<script>var memorial={"firstName":"Robert","lastName":"Lee","birthYear":"18`,
		strings.Repeat("<div><span>", 4000),
		string([]byte{0xff, 0xfe, 0xfd, '<', 'h', 't', 'm', 'l', '>'}),
		`<html><body><div id="family-grid"><ul aria-labelledby="spouseLabel"><li><a href="/memorial/123"></a></li></ul>`,
	}

	for index, input := range cases {
		index, input := index, input
		t.Run(string(rune('A'+index)), func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = findagrave.ParseInput(context.Background(), input)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("parser hung for malformed case %d", index)
			}
		})
	}
}

func FuzzFindAGraveParseHTMLDoesNotPanic(f *testing.F) {
	seeds := []string{
		`<html><body><div id="bio-name">John Example</div></body></html>`,
		`<html><script>firstName:"John",lastName:"Example",birthYear:"1840"</script></html>`,
		`<html><body><div id="family-grid"><ul aria-labelledby="spouseLabel"><li><h3 itemprop="name">Mary Example</h3></li></ul></div></body></html>`,
		string([]byte{0xff, 0xfe, 0xfd}),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = findagrave.ParseHTML(input, "fuzz", "")
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("ParseHTML hung for fuzz input of length %d", len(input))
		}
	})
}
