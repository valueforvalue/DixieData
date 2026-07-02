package appshell

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// TestHandleSharePrintRecordsFragment_RendersRecords is the
// regression net for the lazy-load fragment endpoint registered
// for issue #234. The fragment must:
//   - return 200 with text/html content-type
//   - contain the [data-print-config-body] sentinel wrapper
//   - contain at least one [data-print-record-checkbox]
//   - contain [data-print-filter-family] markers for the 5
//     filter families
//
// Endpoint is hit by htmx when the user opens the print-config
// modal. Failure mode (DB error) is exercised in a separate test.
func TestHandleSharePrintRecordsFragment_RendersRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("fragment test runs without -short")
	}
	app := newStressApp(t)

	for i := 0; i < 50; i++ {
		_, err := app.soldiers.Create(models.Soldier{
			DisplayID: fmt.Sprintf("PRF-%03d", i),
			FirstName: fmt.Sprintf("Print-%d", i),
			LastName:  fmt.Sprintf("Fragment-%d", i),
			Unit:      fmt.Sprintf("Regiment %d", i%5),
		})
		if err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
	}

	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/print-records-fragment")
	if err != nil {
		t.Fatalf("GET fragment: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html prefix", ct)
	}

	body := readBody(t, resp)
	if !strings.Contains(body, "data-print-config-body") {
		t.Fatalf("fragment must wrap content in [data-print-config-body]; body length %d", len(body))
	}
	if !strings.Contains(body, "data-print-record-checkbox") {
		t.Fatalf("fragment must contain per-record checkboxes; body length %d", len(body))
	}
	for _, family := range []string{"buried-in", "entry-type", "unit", "pension-state", "confederate-home-status"} {
		if !strings.Contains(body, fmt.Sprintf(`data-print-filter-family=%q`, family)) {
			t.Fatalf("fragment must contain filter family %q; body length %d", family, len(body))
		}
	}
}

// TestHandleSharePrintRecordsFragment_EmptyArchive asserts the
// fragment renders without crashing when no records exist. The
// fragment templ checks len(exportRecords) == 0 and renders the
// "No records available." placeholder inside data-print-record-options.
func TestHandleSharePrintRecordsFragment_EmptyArchive(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/share/print-records-fragment")
	if err != nil {
		t.Fatalf("GET fragment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "No records available.") {
		preview := body
		if len(preview) > 500 {
			preview = preview[:500]
		}
		t.Fatalf("empty archive must render the no-records placeholder; body: %q", preview)
	}
}

// TestHandleSharePrintRecordsFragment_WrongMethod pins the 405
// contract. The modal opens via user click which fires a GET;
// POSTs land here only via misrouted forms and must fail loudly
// rather than render a fragment.
func TestHandleSharePrintRecordsFragment_WrongMethod(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Post(server.URL+"/share/print-records-fragment", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST fragment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

// readBody drains an http.Response body and returns it as a string.
// Local helper to keep the test focused on assertions, not I/O.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(raw)
}