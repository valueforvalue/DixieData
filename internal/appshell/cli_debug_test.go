// cli_debug_test.go — Phase 7 of cli-plan.md.
//
// Parser tests are pure (no App required). The hx-invariants
// walker + route collector tests construct minimal in-memory
// inputs (synthetic .templ files, synthetic routes.go) so the
// assertions stay deterministic and don't depend on the live
// repo layout. The dump + request tests use a real App
// constructed with a temp data dir so we exercise the SQL +
// dispatch path end-to-end.
package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

func TestHasDebugSubcommand(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"debug"}, true},
		{[]string{"debug", "dump"}, true},
		{[]string{"debug", "hx-invariants"}, true},
		{[]string{"debug", "browser-tree"}, true},
		{[]string{"debug", "request", "/soldiers/1"}, true},
		{[]string{"debug", "frobnicate"}, true}, // caught by parser for clear error
		{[]string{"doctor"}, false},
		{[]string{"list"}, false},
		{[]string{"import", "backup"}, false},
		{[]string{"export", "pdf"}, false},
	}
	for _, tc := range cases {
		if got := HasDebugSubcommand(tc.args); got != tc.want {
			t.Errorf("HasDebugSubcommand(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestParseDebugArgs(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantKind    DebugKind
		wantPath    string
		wantJSON    bool
		wantDataDir string
		wantErr     bool
	}{
		{"empty", nil, DebugUnknown, "", false, "", false},
		{"non-debug", []string{"list"}, DebugUnknown, "", false, "", false},
		{"dump", []string{"debug", "dump"}, DebugDump, "", false, "", false},
		{"dump-json", []string{"debug", "dump", "--json"}, DebugDump, "", true, "", false},
		{"hx", []string{"debug", "hx-invariants"}, DebugHXInvariants, "", false, "", false},
		{"browser-tree", []string{"debug", "browser-tree"}, DebugBrowserTree, "", false, "", false},
		{"request", []string{"debug", "request", "/soldiers/123"}, DebugRequest, "/soldiers/123", false, "", false},
		{"request-no-path", []string{"debug", "request"}, DebugRequest, "", false, "", true},
		{"request-with-json", []string{"debug", "request", "/calendar", "--json"}, DebugRequest, "/calendar", true, "", false},
		{"request-data-dir", []string{"debug", "dump", "--data-dir", "/tmp/data"}, DebugDump, "", false, "/tmp/data", false},
		{"request-data-dir-eq", []string{"debug", "dump", "--data-dir=/tmp/data"}, DebugDump, "", false, "/tmp/data", false},
		{"unknown-kind", []string{"debug", "frobnicate"}, DebugUnknown, "", false, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := ParseDebugArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseDebugArgs(%v) error = nil, want err", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDebugArgs(%v) error = %v", tc.args, err)
			}
			if opts.Kind != tc.wantKind {
				t.Errorf("Kind = %v, want %v", opts.Kind, tc.wantKind)
			}
			if opts.RequestPath != tc.wantPath {
				t.Errorf("RequestPath = %q, want %q", opts.RequestPath, tc.wantPath)
			}
			if opts.JSON != tc.wantJSON {
				t.Errorf("JSON = %v, want %v", opts.JSON, tc.wantJSON)
			}
			if opts.DataDir != tc.wantDataDir {
				t.Errorf("DataDir = %q, want %q", opts.DataDir, tc.wantDataDir)
			}
		})
	}
}

func TestDebugKindString(t *testing.T) {
	cases := map[DebugKind]string{
		DebugDump:         "dump",
		DebugHXInvariants: "hx-invariants",
		DebugBrowserTree:  "browser-tree",
		DebugRequest:      "request",
		DebugUnknown:      "unknown",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", k, got, want)
		}
	}
}

// --- hx-attr walker tests (synthetic fixtures) ---

func TestCollectTemplFiles(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{
		"a.templ",
		"components/b.templ",
		"components/sub/c.templ",
		"readme.md",
		"components/d.go",
	} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte("hello"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	files, err := collectTemplFiles(root)
	if err != nil {
		t.Fatalf("collectTemplFiles: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("len(files) = %d, want 3 (got %v)", len(files), files)
	}
}

func TestCollectTemplFilesMissingRoot(t *testing.T) {
	files, err := collectTemplFiles(filepath.Join(t.TempDir(), "no-such"))
	if err != nil {
		t.Fatalf("err = %v, want nil for missing root", err)
	}
	if len(files) != 0 {
		t.Errorf("len(files) = %d, want 0", len(files))
	}
}

func TestMatchChiPattern(t *testing.T) {
	cases := []struct {
		pattern, value string
		want           bool
	}{
		{"/", "/", true},
		{"/soldiers", "/soldiers", true},
		{"/soldiers/{id}", "/soldiers/123", true},
		{"/soldiers/{id}", "/soldiers", false},
		{"/soldiers/*", "/soldiers/123", true},
		{"/soldiers/*", "/soldiers/123/records", true},
		{"/soldiers/*", "/other", false},
		{"/calendar/{month}/{day}", "/calendar/6/15", true},
		{"/calendar/{month}/{day}", "/calendar/6", false},
		{"/export/{kind:json|csv}", "/export/json", true},
	}
	for _, tc := range cases {
		if got := matchChiPattern(tc.pattern, tc.value); got != tc.want {
			t.Errorf("matchChiPattern(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.want)
		}
	}
}

func TestIsRouteRegistered(t *testing.T) {
	routes := []registeredRoute{
		{Pattern: "/soldiers", Method: "Get"},
		{Pattern: "/soldiers/{id}", Method: "Get"},
		{Pattern: "/calendar/{month}/{day}", Method: "Get"},
		{Pattern: "/export/json", Method: "Post"},
	}
	cases := []struct {
		value string
		want  bool
	}{
		{"/soldiers", true},
		{"/soldiers/123", true},
		{"/calendar/6/15", true},
		{"/export/json", true},
		{"/no-such-route", false},
		{"", true}, // empty is skipped (see comment in isRouteRegistered)
		{"/app.js", true}, // static asset
		{"/soldiers?id=1", true}, // query string stripped
	}
	for _, tc := range cases {
		if got := isRouteRegistered(tc.value, routes); got != tc.want {
			t.Errorf("isRouteRegistered(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestHxAttrRegexCapturesAttributes(t *testing.T) {
	src := `<a hx-get="/foo" hx-target="#bar" hx-post='/baz' >link</a>`
	matches := hxAttrRegex.FindAllStringSubmatch(src, -1)
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want 3 (got %v)", len(matches), matches)
	}
	// First match: hx-get="/foo"
	if matches[0][1] != "hx-get" || matches[0][2] != "/foo" {
		t.Errorf("matches[0] = %v", matches[0])
	}
	// Second match: hx-target="#bar"
	if matches[1][1] != "hx-target" || matches[1][2] != "#bar" {
		t.Errorf("matches[1] = %v", matches[1])
	}
	// Third match: hx-post='/baz' (single quotes)
	if matches[2][1] != "hx-post" || (matches[2][2] != "/baz" && matches[2][3] != "/baz") {
		t.Errorf("matches[2] = %v", matches[2])
	}
}

func TestIdAttrRegexCapturesIds(t *testing.T) {
	src := `<div id="alpha"><span id='beta'>x</span><input id="gamma"></div>`
	matches := idAttrRegex.FindAllStringSubmatch(src, -1)
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want 3 (got %v)", len(matches), matches)
	}
	want := []string{"alpha", "beta", "gamma"}
	for i, m := range matches {
		got := m[1]
		if got == "" {
			got = m[2]
		}
		if got != want[i] {
			t.Errorf("matches[%d] = %q, want %q", i, got, want[i])
		}
	}
}

// --- runDebugHXInvariants against synthetic fixtures ---

func TestRunDebugHXInvariantsClean(t *testing.T) {
	// Build a single synthetic fixture root with BOTH the
	// templ tree AND the routes.go, plus a go.mod so repoRoot()
	// resolves to it. The walker then finds the templates and
	// the routes from the same root.
	root := writeCombinedFixture(t, []string{
		`r.Get("/foo", handle)`,
		`r.Post("/bar", handle)`,
	}, map[string]string{
		"a.templ":            `<div id="main"><a hx-get="/foo" hx-target="#main">x</a></div>`,
		"components/b.templ": `<form hx-post="/bar" hx-target="#main">go</form>`,
	})
	app := &App{dataDir: root}
	code, err := runDebugHXInvariants(context.Background(), app, DebugOptions{
		Writer: io.Discard,
		Now:    func() int64 { return 0 },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0 (clean)", code)
	}
}

func TestRunDebugHXInvariantsMissingTarget(t *testing.T) {
	root := writeCombinedFixture(t, []string{
		`r.Get("/foo", handle)`,
	}, map[string]string{
		"a.templ": `<a hx-get="/foo" hx-target="#no-such-id">x</a>`,
	})
	app := &App{dataDir: root}
	var buf strings.Builder
	code, err := runDebugHXInvariants(context.Background(), app, DebugOptions{
		Writer: &buf,
		Now:    func() int64 { return 0 },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if code != 1 {
		t.Errorf("code = %d, want 1 (invariant failure)", code)
	}
	if !strings.Contains(buf.String(), "missing-target") {
		t.Errorf("output should mention missing-target, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "#no-such-id") {
		t.Errorf("output should include the offending id, got: %s", buf.String())
	}
}

func TestRunDebugHXInvariantsUnregisteredRoute(t *testing.T) {
	root := writeCombinedFixture(t, []string{
		`r.Get("/foo", handle)`,
	}, map[string]string{
		"a.templ": `<a hx-get="/no-such-route">x</a>`,
	})
	app := &App{dataDir: root}
	var buf strings.Builder
	code, _ := runDebugHXInvariants(context.Background(), app, DebugOptions{
		Writer: &buf,
		Now:    func() int64 { return 0 },
	})
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if !strings.Contains(buf.String(), "unregistered-route") {
		t.Errorf("output should mention unregistered-route, got: %s", buf.String())
	}
}

func TestRunDebugHXInvariantsJSONOutput(t *testing.T) {
	root := writeCombinedFixture(t, []string{
		`r.Get("/foo", handle)`,
	}, map[string]string{
		"a.templ": `<a hx-get="/no-such">x</a>`,
	})
	app := &App{dataDir: root}
	var buf strings.Builder
	code, err := runDebugHXInvariants(context.Background(), app, DebugOptions{
		Writer: &buf,
		JSON:   true,
		Now:    func() int64 { return 0 },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	var report HXInvariantsReport
	if err := json.Unmarshal([]byte(buf.String()), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if report.Clean {
		t.Error("report.Clean = true, want false")
	}
	if len(report.Violations) == 0 {
		t.Error("report.Violations empty, want at least 1")
	}
}

// --- browser-tree tests ---

func TestRunDebugBrowserTreeEmpty(t *testing.T) {
	// An App with no mux set returns an empty tree.
	app := &App{}
	var buf strings.Builder
	code, err := runDebugBrowserTree(context.Background(), app, DebugOptions{
		Writer: &buf,
		Now:    func() int64 { return 0 },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
}

func TestRunDebugBrowserTreeASTWalksRoutesFile(t *testing.T) {
	// When dataDir points at a real repo root, the AST walker
	// finds the live routes.go. This test runs from inside
	// internal/appshell, so the repo root is two dirs up.
	app := &App{}
	cwd, _ := os.Getwd()
	app.dataDir = filepath.Join(cwd, "..", "..")
	var buf strings.Builder
	code, err := runDebugBrowserTree(context.Background(), app, DebugOptions{
		Writer: &buf,
		JSON:   true,
		Now:    func() int64 { return 0 },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	var report BrowserTreeReport
	if err := json.Unmarshal([]byte(buf.String()), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if report.RouteCount < 5 {
		t.Errorf("RouteCount = %d, want at least 5 (live routes.go)", report.RouteCount)
	}
	// Live routes.go registers /soldiers/{id} via wildcard;
	// verify at least one wildcard pattern is captured.
	found := false
	for _, r := range report.Routes {
		if strings.Contains(r.Pattern, "{") || strings.Contains(r.Pattern, "*") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no parameterised routes captured; routes: %v", report.Routes)
	}
}

// --- request tests ---

func TestDispatchHeadlessRequestRequiresMux(t *testing.T) {
	app := &App{}
	_, err := app.DispatchHeadlessRequest("/foo")
	if err == nil {
		t.Error("err = nil, want error when mux is nil")
	}
}

func TestDispatchHeadlessRequestEmptyPath(t *testing.T) {
	app := &App{mux: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})}
	_, err := app.DispatchHeadlessRequest("")
	if err == nil {
		t.Error("err = nil, want error when path is empty")
	}
}

func TestDispatchHeadlessRequestNormalisesPath(t *testing.T) {
	// Path without leading slash should be normalised.
	called := ""
	app := &App{
		mux: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = r.URL.Path
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("ok"))
		}),
	}
	report, err := app.DispatchHeadlessRequest("foo/bar")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if called != "/foo/bar" {
		t.Errorf("server saw path %q, want /foo/bar", called)
	}
	if report.Status != 200 {
		t.Errorf("Status = %d, want 200", report.Status)
	}
	if report.Body != "ok" {
		t.Errorf("Body = %q, want ok", report.Body)
	}
	if report.Path != "foo/bar" {
		t.Errorf("Path = %q, want foo/bar (unchanged)", report.Path)
	}
}

func TestDispatchHeadlessRequestTruncatesLargeBody(t *testing.T) {
	big := strings.Repeat("x", 100*1024)
	app := &App{
		mux: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, big)
		}),
	}
	report, err := app.DispatchHeadlessRequest("/big")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !report.BodyTruncated {
		t.Error("BodyTruncated = false, want true")
	}
	if len(report.Body) != 64*1024 {
		t.Errorf("len(Body) = %d, want 64KB", len(report.Body))
	}
}

func TestRunDebugRequestRequiresPath(t *testing.T) {
	app := &App{}
	var buf strings.Builder
	code, err := runDebugRequest(context.Background(), app, DebugOptions{
		Writer: &buf,
		Now:    func() int64 { return 0 },
	})
	if err == nil {
		t.Fatal("err = nil, want error when path is empty")
	}
	if code != 3 {
		t.Errorf("code = %d, want 3 (usage error)", code)
	}
}

// --- dump tests (uses real App via NewApp + temp data dir) ---

func TestArchiveInventoryOnEmptyDB(t *testing.T) {
	app := newHeadlessAppForTest(t)
	inv, err := app.ArchiveInventory()
	if err != nil {
		t.Fatalf("ArchiveInventory: %v", err)
	}
	if inv.DataDir == "" {
		t.Error("DataDir empty")
	}
	if inv.AppVersion == "" {
		t.Error("AppVersion empty")
	}
	if inv.SchemaVersion == 0 {
		t.Error("SchemaVersion = 0, want non-zero (migrations should have run)")
	}
	if inv.RowCounts["soldiers"] != 0 {
		t.Errorf("soldiers count = %d, want 0", inv.RowCounts["soldiers"])
	}
	if inv.RowCounts["records"] != 0 {
		t.Errorf("records count = %d, want 0", inv.RowCounts["records"])
	}
}

func TestArchiveInventoryJSON(t *testing.T) {
	app := newHeadlessAppForTest(t)
	var buf strings.Builder
	code, err := runDebugDump(context.Background(), app, DebugOptions{
		Writer: &buf,
		JSON:   true,
		Now:    func() int64 { return 0 },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	var inv ArchiveInventory
	if err := json.Unmarshal([]byte(buf.String()), &inv); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if inv.SchemaVersion == 0 {
		t.Error("SchemaVersion = 0 in JSON output")
	}
}

func TestArchiveInventoryText(t *testing.T) {
	app := newHeadlessAppForTest(t)
	var buf strings.Builder
	code, err := runDebugDump(context.Background(), app, DebugOptions{
		Writer: &buf,
		Now:    func() int64 { return 0 },
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	s := buf.String()
	for _, want := range []string{
		"Archive Inventory",
		"Schema version",
		"Row counts",
		"Soldiers",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("text output missing %q\nfull output:\n%s", want, s)
		}
	}
}

// --- helpers ---

// newHeadlessAppForTest creates a real App, points it at a temp
// data dir, and runs Startup so the database is open. Used by
// dump + request tests that exercise SQL. We DO call Shutdown via
// t.Cleanup so the temp dir is unlocked.
//
// Windows quirk: the jobs registry opens the jobs log (under
// <dataDir-parent>/.dixiedata-logs/jobs.jsonl) in append mode
// and keeps the handle alive across Shutdown (only the worker
// pool drains — the file handle is owned by the registry,
// which doesn't expose a Close method). We use reflection to
// reach the unexported logCloser field and close it
// explicitly. Reflection is acceptable in test code only; it
// would be a hidden coupling if used in production. The log
// lives outside the data dir on purpose: replaceDataDir
// renames <dataDir> atomically, and an open handle inside the
// data dir blocks the rename on Windows.
func newHeadlessAppForTest(t *testing.T) *App {
	t.Helper()
	tmp, err := os.MkdirTemp("", "dixiedata-debug-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("DIXIEDATA_DATA_DIR", tmp)
	a := NewApp()
	ctx := context.Background()
	a.Startup(ctx)
	t.Cleanup(func() {
		a.Shutdown(ctx)
		closeJobsLogWriter(t, a)
		_ = os.RemoveAll(tmp)
	})
	return a
}

// closeJobsLogWriter reaches into the jobs.Registry to close
// the file handle held by SetLogWriter. We use reflection + a
// safe.Pointer round-trip because the field is unexported and
// there's no public CloseLogWriter helper yet (the registry
// assumes long-lived processes). Test-only — production code
// calls os.Exit after Shutdown so this never matters outside
// tests.
func closeJobsLogWriter(t *testing.T, a *App) {
	t.Helper()
	if a == nil || a.jobs == nil {
		return
	}
	v := reflect.ValueOf(a.jobs).Elem()
	closer := v.FieldByName("logCloser")
	if !closer.IsValid() || closer.IsZero() {
		return
	}
	// reflect.Value.Interface panics on unexported fields; go
	// through unsafe.Pointer to read the value as io.Closer.
	ptr := unsafe.Pointer(closer.UnsafeAddr())
	c := *(*io.Closer)(ptr)
	if c != nil {
		_ = c.Close()
	}
}

// writeCombinedFixture creates one temp root containing BOTH a
// synthetic routes.go AND a synthetic .templ tree, plus a
// go.mod so repoRoot() resolves to it. Returns the root path;
// the test then constructs `&App{dataDir: root}` so the walker
// sees a single coherent fixture (matches the live CLI's view
// where dataDir + repo root live in the same tree).
func writeCombinedFixture(t *testing.T, routeLines []string, templFiles map[string]string) string {
	t.Helper()
	root := t.TempDir()
	// Internal/appshell/routes.go (AST-walked by collectRegisteredRoutes).
	appshellDir := filepath.Join(root, "internal", "appshell")
	if err := os.MkdirAll(appshellDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	contents := `package appshell

import "github.com/go-chi/chi/v5"

func (a *App) syntheticRoutes() {
	r := chi.NewRouter()
` + "\n" + strings.Join(routeLines, "\n") + "\n}\n"
	if err := os.WriteFile(filepath.Join(appshellDir, "routes.go"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write routes: %v", err)
	}
	// go.mod so repoRoot() finds the fixture.
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// Internal/templates/*.templ (walked by collectTemplFiles).
	tplDir := filepath.Join(root, "internal", "templates")
	if err := os.MkdirAll(filepath.Join(tplDir, "components"), 0o755); err != nil {
		t.Fatalf("mkdir templ: %v", err)
	}
	for name, body := range templFiles {
		full := filepath.Join(tplDir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

// writeSyntheticTemplTree creates a temp root/.templ-style file
// layout and returns the path. The structure is:
//
//	<root>/
//	  internal/
//	    templates/
//	      a.templ
//	      components/b.templ
//
// (No .templ generation needed; we hand-write the content.)
func writeSyntheticTemplTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	tplDir := filepath.Join(root, "internal", "templates")
	if err := os.MkdirAll(filepath.Join(tplDir, "components"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range files {
		full := filepath.Join(tplDir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

// silence unused-import lint when httptest is only used by
// helpers added later.
var _ = httptest.NewRecorder
var _ = fmt.Sprintf
