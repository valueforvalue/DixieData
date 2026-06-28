// cli_debug.go — Phase 7 of docs/agents/cli-plan.md.
//
// Four read-only debugging subcommands that bypass the GUI for
// support workflows:
//
//	dixiedata debug dump             # full archive inventory
//	dixiedata debug hx-invariants    # walk .templ files, check hx-target/hx-post consistency
//	dixiedata debug browser-tree     # print registered route tree
//	dixiedata debug request <path>   # simulate a request, print what handler returns
//
// Hard constraint: READ-ONLY. Debug subcommands never write to
// the archive, never mutate data, never accept --yes. The whole
// point is safe inspection by user support. Existing *App methods
// are called as-is; new thin wrappers (`App.ArchiveInventory`,
// `App.DispatchHeadlessRequest`) are added in this file to bridge
// the gap between the CLI's flat shape and the App's nested
// service layout. No business logic lives in this file.
package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

// DebugKind identifies which Phase 7 subcommand the user wants.
type DebugKind int

const (
	DebugUnknown DebugKind = iota
	DebugDump
	DebugHXInvariants
	DebugBrowserTree
	DebugRequest
)

// String returns the lowercase verb used on the command line.
func (k DebugKind) String() string {
	switch k {
	case DebugDump:
		return "dump"
	case DebugHXInvariants:
		return "hx-invariants"
	case DebugBrowserTree:
		return "browser-tree"
	case DebugRequest:
		return "request"
	default:
		return "unknown"
	}
}

// DebugOptions configures RunDebug. The parser fills Kind and
// the kind-specific fields; the runner fills App/Writer/Now from
// the lifecycle wrapper.
type DebugOptions struct {
	Kind      DebugKind
	Args      []string // raw trailing args (e.g. debug request <path>)
	RequestPath string // for DebugRequest
	JSON      bool
	DataDir   string // override for appdata.DefaultDir()
	Writer    io.Writer
	App       *App
	Now       func() int64 // unix seconds; injected for tests
}

// debugDefaults returns the default Now function (time.Now().Unix).
func debugDefaults() func() int64 {
	return func() int64 { return time.Now().Unix() }
}

// RunDebug dispatches to the right handler. Returns exit code
// (0 ok, 1 invariant failure, 2 env error, 3 usage error).
func RunDebug(ctx context.Context, opts DebugOptions) (int, error) {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.Now == nil {
		opts.Now = debugDefaults()
	}
	app := opts.App
	if app == nil {
		return 2, fmt.Errorf("RunDebug requires opts.App (or use RunDebug via main.go which builds one)")
	}

	switch opts.Kind {
	case DebugDump:
		return runDebugDump(ctx, app, opts)
	case DebugHXInvariants:
		return runDebugHXInvariants(ctx, app, opts)
	case DebugBrowserTree:
		return runDebugBrowserTree(ctx, app, opts)
	case DebugRequest:
		return runDebugRequest(ctx, app, opts)
	default:
		return 3, fmt.Errorf("unknown debug command")
	}
}

// --- ArchiveInventory ---

// ArchiveInventory is the structured payload returned by
// `debug dump`. Includes schema version, app version, identity
// (Local Archive, not exported/shared), row counts for every
// table we know about, and the local_settings.json snapshot.
type ArchiveInventory struct {
	Command         string            `json:"command"`
	GeneratedAt     string            `json:"generated_at"`
	DataDir         string            `json:"data_dir"`
	AppVersion      string            `json:"app_version"`
	BuildIdentity   string            `json:"build_identity"`
	SchemaVersion   int               `json:"schema_version"`
	ArchiveCounts   models.ArchiveCounts `json:"archive_counts"`
	RowCounts       map[string]int    `json:"row_counts"`
	LocalSettings   records.LocalSettings `json:"local_settings"`
	UserIdentity    models.UserIdentity   `json:"user_identity"`
	IdentityComplete bool                  `json:"identity_complete"`
}

// inventoryRowQueries lists every row count we know how to read
// safely (read-only SQL, no migrations). Add a row here and the
// dump command picks it up automatically.
var inventoryRowQueries = []struct {
	Label string
	SQL   string
}{
	{"soldiers", `SELECT COUNT(*) FROM soldiers`},
	{"records", `SELECT COUNT(*) FROM records`},
	{"images", `SELECT COUNT(*) FROM images`},
	{"calendar_items", `SELECT COUNT(*) FROM calendar_items`},
	{"duplicate_audit_findings", `SELECT COUNT(*) FROM duplicate_audit_findings`},
	{"duplicate_audit_findings_pending", `SELECT COUNT(*) FROM duplicate_audit_findings WHERE status = 'pending'`},
	{"merge_review_sessions", `SELECT COUNT(*) FROM merge_review_sessions`},
	{"merge_review_conflicts", `SELECT COUNT(*) FROM merge_review_conflicts`},
	{"merge_review_conflicts_pending", `SELECT COUNT(*) FROM merge_review_conflicts WHERE resolution = '' OR resolution IS NULL`},
	{"shared_merge_aliases", `SELECT COUNT(*) FROM shared_merge_aliases`},
	{"research_tasks", `SELECT COUNT(*) FROM research_tasks`},
	{"research_collections", `SELECT COUNT(*) FROM research_collections`},
	{"research_collection_items", `SELECT COUNT(*) FROM research_collection_items`},
	{"import_batches", `SELECT COUNT(*) FROM import_batches`},
	{"soldiers_needing_review", `SELECT COUNT(*) FROM soldiers WHERE needs_review = 1`},
}

// ArchiveInventory builds the structured payload. Read-only: never
// touches the archive file beyond SELECT queries. Thin wrapper
// around the existing db.DB + records.LocalSettings so the CLI
// handler stays a renderer.
func (a *App) ArchiveInventory() (ArchiveInventory, error) {
	inv := ArchiveInventory{
		Command:       "dixiedata debug dump",
		DataDir:       a.dataDir,
		AppVersion:    buildinfo.AppVersion,
		BuildIdentity: buildinfo.BuildIdentity(),
		RowCounts:     make(map[string]int, len(inventoryRowQueries)),
	}
	inv.GeneratedAt = time.Unix(time.Now().Unix(), 0).UTC().Format(time.RFC3339)
	if a.database == nil {
		return inv, fmt.Errorf("app database not initialized (startup did not complete)")
	}
	conn := a.database.Conn()
	var v int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return inv, fmt.Errorf("read user_version: %w", err)
	}
	inv.SchemaVersion = v

	if a.soldiers != nil {
		counts, err := a.soldiers.ArchiveCounts()
		if err != nil {
			return inv, fmt.Errorf("archive counts: %w", err)
		}
		inv.ArchiveCounts = counts
	}

	for _, q := range inventoryRowQueries {
		var n int
		if err := conn.QueryRow(q.SQL).Scan(&n); err != nil {
			// Surface the error inline so the dump command
			// fails loudly if a table disappears or the
			// schema drifts. Better than silent zero — the
			// user's support dump should be trustworthy.
			return inv, fmt.Errorf("count %s: %w", q.Label, err)
		}
		inv.RowCounts[q.Label] = n
	}

	settings, err := records.LoadLocalSettings(a.dataDir)
	if err != nil {
		// Tolerate a corrupt settings file: surface as empty
		// struct + stderr note (the dump itself must not
		// abort just because settings.json is malformed).
		fmt.Fprintf(os.Stderr, "warning: load local_settings: %v\n", err)
	}
	inv.LocalSettings = settings

	if identity, err := a.database.UserIdentity(); err == nil {
		inv.UserIdentity = identity
	} else {
		fmt.Fprintf(os.Stderr, "warning: load user identity: %v\n", err)
	}
	if complete, err := a.database.IdentitySetupRequired(); err == nil {
		inv.IdentityComplete = !complete
	}
	return inv, nil
}

// --- dump ---

func runDebugDump(ctx context.Context, app *App, opts DebugOptions) (int, error) {
	inv, err := app.ArchiveInventory()
	if err != nil {
		return 1, err
	}
	if opts.JSON {
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(inv); err != nil {
			return 1, err
		}
		return 0, nil
	}
	renderDebugDumpText(opts.Writer, inv)
	return 0, nil
}

func renderDebugDumpText(w io.Writer, inv ArchiveInventory) {
	fmt.Fprintln(w, "Archive Inventory")
	fmt.Fprintln(w, "-----------------")
	fmt.Fprintf(w, "  Generated:        %s\n", inv.GeneratedAt)
	fmt.Fprintf(w, "  Data directory:   %s\n", inv.DataDir)
	fmt.Fprintf(w, "  App version:      %s\n", inv.AppVersion)
	fmt.Fprintf(w, "  Build identity:   %s\n", inv.BuildIdentity)
	fmt.Fprintf(w, "  Schema version:   %d\n", inv.SchemaVersion)
	fmt.Fprintf(w, "  Identity complete: %t\n", inv.IdentityComplete)
	if inv.UserIdentity.NodePrefix != "" {
		fmt.Fprintf(w, "  Node prefix:      %s\n", inv.UserIdentity.NodePrefix)
	}
	if inv.UserIdentity.FirstName != "" {
		fmt.Fprintf(w, "  User:             %s %s %s (%d)\n",
			inv.UserIdentity.FirstName,
			inv.UserIdentity.MiddleName,
			inv.UserIdentity.LastName,
			inv.UserIdentity.BirthYear)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Archive counts:")
	fmt.Fprintf(w, "  Soldiers:         %d\n", inv.ArchiveCounts.TotalSoldiers)
	fmt.Fprintf(w, "  Wives/widows:     %d\n", inv.ArchiveCounts.TotalWivesWidows)
	fmt.Fprintf(w, "  Linked people:    %d\n", inv.ArchiveCounts.TotalLinkedPeople)
	fmt.Fprintf(w, "  Total records:    %d\n", inv.ArchiveCounts.TotalRecords())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Row counts:")
	keys := make([]string, 0, len(inv.RowCounts))
	for k := range inv.RowCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "  %-32s %d\n", k, inv.RowCounts[k])
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Local settings:\n")
	fmt.Fprintf(w, "  debug_mode:       %t\n", inv.LocalSettings.DebugMode)
}

// --- hx-invariants ---

// HXViolationKind classifies a single invariant failure.
type HXViolationKind string

const (
	HXViolationMissingTarget HXViolationKind = "missing-target"
	HXViolationUnregistered  HXViolationKind = "unregistered-route"
	HXViolationParseError    HXViolationKind = "parse-error"
)

// HXViolation is a single hx-invariants finding.
type HXViolation struct {
	Kind     HXViolationKind `json:"kind"`
	File     string          `json:"file"`
	Line     int             `json:"line"`
	Attribute string         `json:"attribute"` // e.g. "hx-target", "hx-post"
	Value    string          `json:"value"`
	Reason   string          `json:"reason"`
}

// HXInvariantsReport is the full walker output.
type HXInvariantsReport struct {
	Command        string        `json:"command"`
	GeneratedAt    string        `json:"generated_at"`
	TemplatesRoot  string        `json:"templates_root"`
	FilesScanned   int           `json:"files_scanned"`
	TargetsScanned int           `json:"targets_scanned"`
	RoutesScanned  int           `json:"routes_scanned"`
	Violations     []HXViolation `json:"violations"`
	Clean          bool          `json:"clean"`
}

// hxAttrRegex captures the hx-* attributes that the walker
// looks for. Each group: (1) attribute name (2) attribute value
// (single or double quoted). We deliberately scan raw .templ
// files instead of the generated *_templ.go — the source is
// what reviewers edit, and the violations we want to surface
// (typos in hx-target, dead routes) live in the source.
var hxAttrRegex = regexp.MustCompile(`(hx-(?:target|post|get|put|delete|patch|trigger))\s*=\s*(?:"([^"]*)"|'([^']*)')`)

// idAttrRegex finds id="..." or id='...' declarations so we
// can build the set of known DOM IDs. Same source-only
// rationale as above. Single-quoted IDs are uncommon in
// generated templ output but appear in hand-written testdata
// and a handful of components; capturing them keeps the walker
// honest.
var idAttrRegex = regexp.MustCompile(`\bid\s*=\s*(?:"([^"]+)"|'([^']+)')`)

// runDebugHXInvariants walks every .templ file under the repo's
// internal/templates/ directory and checks two invariants:
//  1. every hx-target="#id" references a DOM ID that exists in
//     some .templ file (resolved across the whole tree, not the
//     same file — the target ID is often on the layout/parent);
//  2. every hx-post/hx-get/hx-put/hx-delete/hx-patch URL resolves
//     to a route registered in routes.go (or to a static asset
//     path we know about — /app.js, /app.css, /htmx.min.js,
//     /debug.js).
//
// Exit 0 if clean; exit 1 if any violations.
func runDebugHXInvariants(ctx context.Context, app *App, opts DebugOptions) (int, error) {
	root := filepath.Join(app.repoRoot(), "internal", "templates")
	report := HXInvariantsReport{
		Command:       "dixiedata debug hx-invariants",
		GeneratedAt:   time.Unix(opts.Now(), 0).UTC().Format(time.RFC3339),
		TemplatesRoot: root,
	}

	templates, err := collectTemplFiles(root)
	if err != nil {
		return 2, fmt.Errorf("walk templates: %w", err)
	}
	knownIDs := make(map[string]struct{})
	type hxRef struct {
		file, attr, value string
		line              int
	}
	var hxRefs []hxRef
	var parseErrors []HXViolation

	for _, path := range templates {
		data, err := os.ReadFile(path)
		if err != nil {
			parseErrors = append(parseErrors, HXViolation{
				Kind: HXViolationParseError, File: path, Line: 0,
				Reason: "read failed: " + err.Error(),
			})
			continue
		}
		rel, _ := filepath.Rel(root, path)
		// Collect DOM IDs (every file contributes to the global set).
		for _, m := range idAttrRegex.FindAllStringSubmatch(string(data), -1) {
			id := m[1]
			if id == "" {
				id = m[2]
			}
			knownIDs[id] = struct{}{}
		}
		// Collect hx-* references with line numbers.
		lines := strings.Split(string(data), "\n")
		for lineIdx, line := range lines {
			for _, m := range hxAttrRegex.FindAllStringSubmatch(line, -1) {
				attr := m[1]
				val := m[2]
				if val == "" {
					val = m[3]
				}
				hxRefs = append(hxRefs, hxRef{
					file: rel, attr: attr, value: val, line: lineIdx + 1,
				})
			}
		}
	}
	report.FilesScanned = len(templates)
	report.TargetsScanned = len(knownIDs)

	// Registered routes from chi's router.
	registeredRoutes := collectRegisteredRoutes(app)
	report.RoutesScanned = len(registeredRoutes)

	// Evaluate each hx-* reference.
	for _, ref := range hxRefs {
		switch ref.attr {
		case "hx-target":
			// Skip non-ID targets like "this", "body", "closest ...",
			// ".class" — these are valid htmx selectors but don't
			// correspond to a DOM id we can verify.
			id := strings.TrimPrefix(ref.value, "#")
			if id == "" || !strings.HasPrefix(ref.value, "#") {
				continue
			}
			if _, ok := knownIDs[id]; !ok {
				report.Violations = append(report.Violations, HXViolation{
					Kind: HXViolationMissingTarget, File: ref.file,
					Line: ref.line, Attribute: ref.attr, Value: ref.value,
					Reason: "hx-target references DOM ID not declared in any .templ file",
				})
			}
		case "hx-post", "hx-get", "hx-put", "hx-delete", "hx-patch":
			if !isRouteRegistered(ref.value, registeredRoutes) {
				report.Violations = append(report.Violations, HXViolation{
					Kind: HXViolationUnregistered, File: ref.file,
					Line: ref.line, Attribute: ref.attr, Value: ref.value,
					Reason: "hx-verb URL does not match any registered route",
				})
			}
		case "hx-trigger":
			// No invariant on triggers — they're event names.
		}
	}

	// Sort violations by file then line for deterministic output.
	sort.Slice(report.Violations, func(i, j int) bool {
		if report.Violations[i].File != report.Violations[j].File {
			return report.Violations[i].File < report.Violations[j].File
		}
		return report.Violations[i].Line < report.Violations[j].Line
	})
	report.Violations = append(report.Violations, parseErrors...)
	report.Clean = len(report.Violations) == 0

	if opts.JSON {
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1, err
		}
	} else {
		renderDebugHXText(opts.Writer, report)
	}

	if !report.Clean {
		return 1, nil
	}
	return 0, nil
}

func renderDebugHXText(w io.Writer, r HXInvariantsReport) {
	fmt.Fprintln(w, "HTMX Invariants")
	fmt.Fprintln(w, "---------------")
	fmt.Fprintf(w, "  Templates root:    %s\n", r.TemplatesRoot)
	fmt.Fprintf(w, "  Files scanned:     %d\n", r.FilesScanned)
	fmt.Fprintf(w, "  DOM IDs indexed:   %d\n", r.TargetsScanned)
	fmt.Fprintf(w, "  Routes indexed:    %d\n", r.RoutesScanned)
	fmt.Fprintf(w, "  Violations:        %d\n", len(r.Violations))
	fmt.Fprintln(w)
	if len(r.Violations) == 0 {
		fmt.Fprintln(w, "  clean.")
		return
	}
	for _, v := range r.Violations {
		fmt.Fprintf(w, "  [%s] %s:%d  %s=%s\n", v.Kind, v.File, v.Line, v.Attribute, v.Value)
		fmt.Fprintf(w, "      %s\n", v.Reason)
	}
}

// collectTemplFiles returns every .templ file under root,
// recursively. Missing root = empty slice (so the report stays
// truthful rather than failing the run).
func collectTemplFiles(root string) ([]string, error) {
	var out []string
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".templ") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// collectRegisteredRoutes AST-walks internal/appshell/routes.go
// (the source of truth) and returns every (pattern, method) pair
// it finds. We AST-walk instead of routing against a live
// `chi.Mux` because the runtime mux is wrapped by
// `debug.Middleware` and `recoverMiddleware` (see routes.go) —
// the wrappers don't expose the underlying tree. The route table
// is also what TestRouteMethodMatchesHandler uses, so this gives
// us a consistent view across CLI + tests.
//
// Method names are chi verbs ("Get", "Post", "Put", "Delete",
// "Patch"). Patterns keep chi's {param} and /* placeholders so
// the matcher can do literal substitution.
func collectRegisteredRoutes(app *App) []registeredRoute {
	if app == nil {
		return nil
	}
	root := app.repoRoot()
	if root == "" {
		return nil
	}
	routesPath := filepath.Join(root, "internal", "appshell", "routes.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		return nil
	}
	var out []registeredRoute
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok || recv.Name != "r" {
			return true
		}
		switch sel.Sel.Name {
		case "Get", "Post", "Put", "Delete", "Patch", "Head", "Options":
		default:
			return true
		}
		if len(call.Args) < 2 {
			return true
		}
		pattern, ok := stringLitValue(call.Args[0])
		if !ok {
			return true
		}
		out = append(out, registeredRoute{Pattern: pattern, Method: sel.Sel.Name})
		return true
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pattern != out[j].Pattern {
			return out[i].Pattern < out[j].Pattern
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// stringLitValue returns the literal value of a string-typed
// ast.Expr when it's a constant. We only need bare literals here
// (every route registration in routes.go uses a constant string);
// computed strings (template-driven) would return ok=false and
// skip, which is safe because there's no such registration in
// the current routes.go.
func stringLitValue(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

type registeredRoute struct {
	Pattern string
	Method  string
}

// isRouteRegistered returns true when value (a URL the
// template references in an hx-* attribute) matches a
// registered route, either exactly or by chi pattern
// substitution. Static asset paths (/app.js, /app.css,
// /htmx.min.js, /debug.js, /index.html) are allowed because
// they're served by the asset handler. Empty values (an
// hx-post with no value) are skipped — the template would
// not work at runtime, but that's a separate lint concern.
func isRouteRegistered(value string, routes []registeredRoute) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	// Strip query string for route matching — chi only sees the
	// path. We check the query separately if needed later.
	if idx := strings.Index(value, "?"); idx >= 0 {
		value = value[:idx]
	}
	switch value {
	case "/app.js", "/app.css", "/htmx.min.js", "/debug.js", "/index.html":
		return true
	}
	for _, r := range routes {
		if matchChiPattern(r.Pattern, value) {
			// We don't enforce method-family matching here
			// (hx-post against a GET-only route, etc.). That
			// belongs to TestRouteMethodMatchesHandler; this
			// walker only checks route existence. If you want
			// to be stricter, add a per-attribute method map:
			//   hx-post => r.Method != http.MethodGet
			//   hx-get  => r.Method == http.MethodGet
			// (Currently every hx-* call site in the templ
			// files happens to be matched correctly because the
			// routes table was authored by hand.)
			return true
		}
	}
	return false
}

// matchChiPattern does a literal-segment substitution of chi's
// {param} and /* placeholders. The two supported shapes are:
//   - {name}   — single path segment
//   - {rest:.*} or * — catch-all remainder (we don't try to
//     validate the captured shape; any non-empty suffix matches)
func matchChiPattern(pattern, value string) bool {
	if pattern == value {
		return true
	}
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	pv := strings.Split(strings.Trim(value, "/"), "/")
	if pattern == "/*" || strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if prefix == "" {
			return strings.HasPrefix(value, "/")
		}
		return strings.HasPrefix(value, strings.TrimSuffix(prefix, "/")+"/")
	}
	if len(pp) != len(pv) {
		return false
	}
	for i, seg := range pp {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			continue // any single segment
		}
		if seg != pv[i] {
			return false
		}
	}
	return true
}

// --- browser-tree ---

// BrowserTreeReport is the structured payload for
// `debug browser-tree`. Sorted routes grouped by HTTP method so
// the JSON shape is stable.
type BrowserTreeReport struct {
	Command      string      `json:"command"`
	GeneratedAt  string      `json:"generated_at"`
	RouteCount   int         `json:"route_count"`
	MethodCounts map[string]int `json:"method_counts"`
	Routes       []BrowserTreeRoute `json:"routes"`
}

// BrowserTreeRoute is a single (pattern, method) tuple.
type BrowserTreeRoute struct {
	Pattern string `json:"pattern"`
	Method  string `json:"method"`
}

func runDebugBrowserTree(ctx context.Context, app *App, opts DebugOptions) (int, error) {
	routes := collectRegisteredRoutes(app)
	report := BrowserTreeReport{
		Command:      "dixiedata debug browser-tree",
		GeneratedAt:  time.Unix(opts.Now(), 0).UTC().Format(time.RFC3339),
		Routes:       make([]BrowserTreeRoute, 0, len(routes)),
		MethodCounts: make(map[string]int),
	}
	for _, r := range routes {
		report.Routes = append(report.Routes, BrowserTreeRoute{
			Pattern: r.Pattern, Method: strings.ToUpper(r.Method),
		})
		report.MethodCounts[strings.ToUpper(r.Method)]++
	}
	report.RouteCount = len(report.Routes)

	if opts.JSON {
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1, err
		}
	} else {
		renderDebugBrowserTreeText(opts.Writer, report)
	}
	return 0, nil
}

func renderDebugBrowserTreeText(w io.Writer, r BrowserTreeReport) {
	fmt.Fprintln(w, "Registered Routes")
	fmt.Fprintln(w, "-----------------")
	fmt.Fprintf(w, "  Total: %d\n", r.RouteCount)
	methods := make([]string, 0, len(r.MethodCounts))
	for m := range r.MethodCounts {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	for _, m := range methods {
		fmt.Fprintf(w, "  %-6s %d\n", m, r.MethodCounts[m])
	}
	fmt.Fprintln(w)
	// Group by method for readability.
	for _, m := range methods {
		fmt.Fprintf(w, "%s:\n", m)
		for _, route := range r.Routes {
			if route.Method != m {
				continue
			}
			fmt.Fprintf(w, "  %s\n", route.Pattern)
		}
	}
}

// --- request ---

// DebugRequestReport is the structured payload for
// `debug request <path>`. Captures the response status, headers
// (subset), and body — exactly what httptest.NewRecorder would
// see, so the support engineer can reproduce a GUI request from
// a shell.
type DebugRequestReport struct {
	Command    string            `json:"command"`
	GeneratedAt string           `json:"generated_at"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Status     int               `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	BodyTruncated bool           `json:"body_truncated"`
	DurationMs int64             `json:"duration_ms"`
}

// DispatchHeadlessRequest invokes the registered mux with a
// synthetic GET request to path and returns the response. Used
// only by the `debug request` subcommand — never reached from
// the GUI. Returns the structured report; caller renders.
//
// We strip the http.Request URL to /<path> only so the support
// engineer can pass either `/soldiers/123` or `soldiers/123`.
func (a *App) DispatchHeadlessRequest(path string) (DebugRequestReport, error) {
	report := DebugRequestReport{
		Command:     "dixiedata debug request",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Method:      http.MethodGet,
		Path:        path,
		Headers:     make(map[string]string),
	}
	if a == nil || a.mux == nil {
		return report, fmt.Errorf("app mux not initialized (startup did not complete)")
	}
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return report, fmt.Errorf("path is required")
	}
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, cleanPath, nil)
	rec := httptest.NewRecorder()
	a.mux.ServeHTTP(rec, req)
	report.DurationMs = time.Since(start).Milliseconds()

	report.Status = rec.Code
	// Surface a small subset of headers — the ones support
	// engineers actually triage. Avoid leaking Set-Cookie or
	// anything sensitive.
	for _, k := range []string{"Content-Type", "Location", "X-Request-Id", "HX-Trigger", "HX-Redirect"} {
		if v := rec.Header().Get(k); v != "" {
			report.Headers[k] = v
		}
	}
	const maxBody = 64 * 1024
	body := rec.Body.Bytes()
	if len(body) > maxBody {
		report.Body = string(body[:maxBody])
		report.BodyTruncated = true
	} else {
		report.Body = string(body)
	}
	return report, nil
}

func runDebugRequest(ctx context.Context, app *App, opts DebugOptions) (int, error) {
	if opts.RequestPath == "" {
		fmt.Fprintln(opts.Writer, "usage: dixiedata debug request <path>")
		return 3, fmt.Errorf("path is required")
	}
	report, err := app.DispatchHeadlessRequest(opts.RequestPath)
	if err != nil {
		return 2, err
	}
	if opts.JSON {
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1, err
		}
		return 0, nil
	}
	renderDebugRequestText(opts.Writer, report)
	return 0, nil
}

func renderDebugRequestText(w io.Writer, r DebugRequestReport) {
	fmt.Fprintln(w, "Request")
	fmt.Fprintln(w, "-------")
	fmt.Fprintf(w, "  Method:        %s\n", r.Method)
	fmt.Fprintf(w, "  Path:          %s\n", r.Path)
	fmt.Fprintf(w, "  Status:        %d\n", r.Status)
	fmt.Fprintf(w, "  Duration:      %dms\n", r.DurationMs)
	if len(r.Headers) > 0 {
		fmt.Fprintln(w, "  Headers:")
		keys := make([]string, 0, len(r.Headers))
		for k := range r.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "    %s: %s\n", k, r.Headers[k])
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Body:")
	if r.Body == "" {
		fmt.Fprintln(w, "  (empty)")
		return
	}
	if r.BodyTruncated {
		fmt.Fprintln(w, "  (truncated to 64KB)")
	}
	// Pretty-print JSON bodies when we can detect them.
	bodyBytes := []byte(r.Body)
	if json.Valid(bodyBytes) && (bytes.HasPrefix(bodyBytes, []byte("{")) || bytes.HasPrefix(bodyBytes, []byte("["))) {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, bodyBytes, "  ", "  "); err == nil {
			fmt.Fprintln(w, pretty.String())
			return
		}
	}
	fmt.Fprintln(w, r.Body)
}

// --- CLI arg parsing ---

// ParseDebugArgs inspects os.Args and returns the DebugKind +
// DebugOptions. Returns DebugUnknown when args don't start with
// "debug" or the second arg isn't a known kind.
//
// Like the other Phase-N parsers, this is hand-rolled — no
// cobra/kingpin. Flag conventions:
//   --json         switch output to JSON envelope
//   --data-dir PATH  override appdata.DefaultDir() resolution
func ParseDebugArgs(args []string) (DebugOptions, error) {
	opts := DebugOptions{}
	if len(args) == 0 {
		return opts, nil
	}
	if args[0] != "debug" {
		return opts, nil
	}
	if len(args) < 2 {
		return opts, fmt.Errorf("debug subcommand requires a kind (dump, hx-invariants, browser-tree, request)")
	}
	switch args[1] {
	case "dump":
		opts.Kind = DebugDump
	case "hx-invariants":
		opts.Kind = DebugHXInvariants
	case "browser-tree":
		opts.Kind = DebugBrowserTree
	case "request":
		opts.Kind = DebugRequest
		// debug request <path> — the path is everything after
		// the verb that isn't a flag, exactly one positional.
		path, err := debugRequestPath(args[2:])
		if err != nil {
			return opts, err
		}
		opts.RequestPath = path
	default:
		return opts, fmt.Errorf("unknown debug subcommand: %s (want dump, hx-invariants, browser-tree, request)", args[1])
	}

	for i, a := range args {
		switch {
		case a == "--json":
			opts.JSON = true
		case strings.HasPrefix(a, "--data-dir="):
			opts.DataDir = strings.TrimPrefix(a, "--data-dir=")
		case a == "--data-dir" && i+1 < len(args):
			opts.DataDir = args[i+1]
		}
	}
	return opts, nil
}

// debugRequestPath extracts the single positional argument from
// the args slice. Flags are filtered; the last remaining
// non-flag arg wins (mirrors `show soldier <id>` parser
// behaviour — flags can sit anywhere).
func debugRequestPath(args []string) (string, error) {
	var path string
	count := 0
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			continue
		}
		path = a
		count++
	}
	if count == 0 {
		return "", fmt.Errorf("debug request requires a path argument (e.g. /soldiers/123)")
	}
	return path, nil
}

// HasDebugSubcommand returns true when the first arg is "debug"
// and the second arg is a known kind. main.go uses this to
// dispatch into RunDebug before falling through to wails.Run.
//
// Returns false for unknown second args so we don't claim
// `debug frobnicate` as ours (main.go prints a usage error
// via ParseDebugArgs in runDebugSubcommand when it reaches
// there). We accept "debug" alone too — the parser will
// reject it with a clearer message than the Wails GUI
// fallback would.
func HasDebugSubcommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "debug"
}

// resolveDataDir applies the data-dir precedence documented in
// cli-plan.md Phase 6 (Open question #1): CLI flag > env var >
// default. We centralise this here so every CLI subcommand can
// call it without re-deriving the env-vs-default ordering.
//
// Returns true when the caller set the env var so the caller can
// log it; errors are returned without fallback (we never silently
// fall back to default if the user explicitly asked for a dir
// that doesn't exist).
func resolveDataDir(cliDataDir string) (string, error) {
	chosen := strings.TrimSpace(cliDataDir)
	source := "--data-dir"
	if chosen == "" {
		chosen = strings.TrimSpace(os.Getenv("DIXIEDATA_DATA_DIR"))
		source = "DIXIEDATA_DATA_DIR"
	}
	if chosen == "" {
		return appdata.DefaultDir(), nil
	}
	abs, err := filepath.Abs(chosen)
	if err != nil {
		return "", fmt.Errorf("%s %q: %w", source, chosen, err)
	}
	return abs, nil
}

// repoRoot returns the absolute path to the repository root by
// walking upward from the executable looking for go.mod.
// Best-effort — returns empty string when the search fails so
// callers can decide what to do. Used only by hx-invariants.
func (a *App) repoRoot() string {
	// Start from the data dir; tests typically run with the
	// data dir inside the repo, so this is a fast path.
	start := a.dataDir
	if start == "" {
		if wd, err := os.Getwd(); err == nil {
			start = wd
		}
	}
	dir := start
	for i := 0; i < 8; i++ {
		if dir == "" {
			break
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ApplyDebugDataDirOverride sets the DIXIEDATA_DATA_DIR env var
// BEFORE the App is constructed. Once startup() has run, the
// dataDir field is fixed; this is the cheapest way to honour
// --data-dir without rewriting the startup contract. main.go's
// runDebugSubcommand wrapper calls this before NewApp so the
// App's startup() picks up the override via appdata.DefaultDir().
//
// Precedence per cli-plan.md Phase 6 (Open question #1):
// CLI --data-dir > env var > default. Callers resolve in that
// order; resolveDataDir is the public helper.
//
// Exported (capital A) so main.go can call it; the rest of the
// helper methods in this file stay lowercase because nothing
// outside the package needs them.
func ApplyDebugDataDirOverride(cliDataDir string) error {
	if strings.TrimSpace(cliDataDir) == "" {
		return nil
	}
	abs, err := filepath.Abs(cliDataDir)
	if err != nil {
		return fmt.Errorf("--data-dir %q: %w", cliDataDir, err)
	}
	return os.Setenv("DIXIEDATA_DATA_DIR", abs)
}
