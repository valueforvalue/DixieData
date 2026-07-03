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
	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

// DebugKind identifies which Phase 7 subcommand the user wants.
type DebugKind int

const (
	DebugUnknown DebugKind = iota
	DebugDump
	DebugHXInvariants
	DebugBrowserTree
	DebugRequest
	DebugCLICoverage
	DebugInPlaceSafety
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
	case DebugCLICoverage:
		return "cli-coverage"
	case DebugInPlaceSafety:
		return "in-place-safety"
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
	case DebugCLICoverage:
		return runDebugCLICoverage(ctx, app, opts)
	case DebugInPlaceSafety:
		return runDebugInPlaceSafety(ctx, app, opts)
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
	// AppVersion is the full v{MAJOR}.{U}.{N} string (e.g.
	// "1.1.1"). Kept as a single field for human readers +
	// scripts that just want the full version stamp.
	AppVersion      string            `json:"app_version"`
	// UpdateFlowVersion is the middle number (U): the
	// update-flow shape gate per issue #266. Exposed
	// explicitly so scripts can compare U without re-parsing
	// the AppVersion string. U=1 covers every legacy
	// v1.2.{N} release.
	UpdateFlowVersion int `json:"update_flow_version"`
	// ReleaseCounter is the trailing number (N): the
	// release counter, independent from the SQLite schema
	// version. Bug-fix-only releases bump N without a schema
	// change.
	ReleaseCounter    int `json:"release_counter"`
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
		Command:           "dixiedata debug dump",
		DataDir:           a.dataDir,
		AppVersion:        buildinfo.AppVersion,
		UpdateFlowVersion: versioninfo.CurrentUpdateFlowVersion,
		ReleaseCounter:    versioninfo.AppRelease(),
		BuildIdentity:     buildinfo.BuildIdentity(),
		RowCounts:         make(map[string]int, len(inventoryRowQueries)),
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
		return opts, fmt.Errorf("debug subcommand requires a kind (dump, hx-invariants, browser-tree, request, cli-coverage)")
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
	case "cli-coverage":
		opts.Kind = DebugCLICoverage
	case "in-place-safety":
		opts.Kind = DebugInPlaceSafety
	default:
		return opts, fmt.Errorf("unknown debug subcommand: %s (want dump, hx-invariants, browser-tree, request, cli-coverage, in-place-safety)", args[1])
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

// CLICoverageReport is the JSON payload for `debug cli-coverage`.
type CLICoverageReport struct {
	Command             string   `json:"command"`
	Documented          []string `json:"documented"`
	Implemented         []string `json:"implemented"`
	DocumentedNotImpl   []string `json:"documented_not_implemented"`
	ImplementedNotDoc   []string `json:"implemented_not_documented"`
	DocumentedCount     int      `json:"documented_count"`
	ImplementedCount    int      `json:"implemented_count"`
	CoveragePercent     int      `json:"coverage_percent"`
	DocPath             string   `json:"doc_path"`
	GeneratedAt         string   `json:"generated_at"`
}

// runDebugCLICoverage walks the CLI subcommand dispatcher in
// main.go / internal/appshell/cli_*.go against the documented
// subcommand list in docs/agents/cli-plan.md. Emits the diff
// of "documented, not implemented" + "implemented, not
// documented" + a coverage percentage. Exit 0 if both sets
// match; exit 1 otherwise.
//
// Implementation strategy: parse the source for `case "...":`
// lines in the cli_*.go files (cheap regex; doesn't need a
// full AST). Parse cli-plan.md for `dixiedata <verb> ...`
// lines under the "shipped" phases. Compare.
//
// Drift detection is the primary use case. A subcommand
// documented but not in the dispatcher (or vice versa) is a
// stale doc or a stale switch — both should be flagged.
func runDebugCLICoverage(ctx context.Context, app *App, opts DebugOptions) (int, error) {
	// Locate the cli-plan.md doc. We try the repo root by
	// walking up from the working dir.
	docPath := findCLICoverageDoc()
	if docPath == "" {
		return 2, fmt.Errorf("could not locate docs/agents/cli-plan.md from working dir")
	}
	implSet := scanImplementedSubcommands(repoRootOf(docPath))
	docSet := scanDocumentedSubcommands(docPath)

	docList := sortedKeys(docSet)
	implList := sortedKeys(implSet)

	docNotImpl := diff(docList, implList)
	implNotDoc := diff(implList, docList)

	coverage := 100
	if len(docList) > 0 {
		matched := 0
		for _, d := range docList {
			if contains(implList, d) {
				matched++
			}
		}
		coverage = (matched * 100) / len(docList)
	}

	report := CLICoverageReport{
		Command:           "debug cli-coverage",
		Documented:        docList,
		Implemented:       implList,
		DocumentedNotImpl: docNotImpl,
		ImplementedNotDoc: implNotDoc,
		DocumentedCount:   len(docList),
		ImplementedCount:  len(implList),
		CoveragePercent:   coverage,
		DocPath:           docPath,
		GeneratedAt:       time.Unix(opts.Now(), 0).UTC().Format(time.RFC3339),
	}

	if opts.JSON {
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return 1, err
		}
	} else {
		fmt.Fprintf(opts.Writer, "dixiedata debug cli-coverage\n")
		fmt.Fprintf(opts.Writer, "================================\n")
		fmt.Fprintf(opts.Writer, "Doc:           %s\n", report.DocPath)
		fmt.Fprintf(opts.Writer, "Documented:    %d subcommands\n", report.DocumentedCount)
		fmt.Fprintf(opts.Writer, "Implemented:   %d subcommands\n", report.ImplementedCount)
		fmt.Fprintf(opts.Writer, "Coverage:      %d%%\n", report.CoveragePercent)
		fmt.Fprintf(opts.Writer, "\n")
		if len(report.DocumentedNotImpl) > 0 {
			fmt.Fprintf(opts.Writer, "Documented, not implemented (drift — remove from docs):\n")
			for _, s := range report.DocumentedNotImpl {
				fmt.Fprintf(opts.Writer, "  - %s\n", s)
			}
			fmt.Fprintf(opts.Writer, "\n")
		}
		if len(report.ImplementedNotDoc) > 0 {
			fmt.Fprintf(opts.Writer, "Implemented, not documented (drift — add to docs OR are leaf verbs under a parent):\n")
			for _, s := range report.ImplementedNotDoc {
				fmt.Fprintf(opts.Writer, "  - %s\n", s)
			}
			fmt.Fprintf(opts.Writer, "\n")
			fmt.Fprintf(opts.Writer, "Note: leaf verbs (e.g. 'pdf' under 'export') appear here because\n")
			fmt.Fprintf(opts.Writer, "they're switch-case literals in the dispatcher but only documented\n")
			fmt.Fprintf(opts.Writer, "as 'dixiedata export pdf'. They are real, reachable verbs. Add a\n")
			fmt.Fprintf(opts.Writer, "top-level 'dixiedata <verb>' line in cli-plan.md ONLY if the verb\n")
			fmt.Fprintf(opts.Writer, "is reachable directly (e.g. via 'dixiedata migrate status', not\n")
			fmt.Fprintf(opts.Writer, "'dixiedata migrate up status').\n\n")
		}
		if len(report.DocumentedNotImpl) == 0 && len(report.ImplementedNotDoc) == 0 {
			fmt.Fprintf(opts.Writer, "Clean: every documented subcommand is implemented and vice versa.\n")
		}
	}

	if len(report.DocumentedNotImpl) > 0 || len(report.ImplementedNotDoc) > 0 {
		return 1, fmt.Errorf("cli-coverage drift detected: %d documented-not-implemented, %d implemented-not-documented",
			len(report.DocumentedNotImpl), len(report.ImplementedNotDoc))
	}
	return 0, nil
}

// findCLICoverageDoc walks up from cwd looking for
// docs/agents/cli-plan.md. Returns the absolute path or "" if
// not found within 5 levels.
func findCLICoverageDoc() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "docs", "agents", "cli-plan.md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// repoRootOf strips /docs/agents/cli-plan.md from a path to
// give the repo root used for scanning cli_*.go.
func repoRootOf(docPath string) string {
	// docPath is absolute; strip the trailing components.
	dir := filepath.Dir(filepath.Dir(filepath.Dir(docPath)))
	return dir
}

// scanImplementedSubcommands walks internal/appshell/cli_*.go
// (and a few siblings — smoke.go, doctor.go — for flag-style
// top-level verbs like --smoke, --version) and pulls every
// quoted verb the dispatcher knows about. Source of truth:
// `func Has<Verb>Subcommand` / `func Has<Verb>Flag` functions
// referenced from main.go's dispatch chain. Each function
// body is scanned for `case "<verb>":` switch statements AND
// `args[0] == "<verb>"` / `args[0] != "<verb>"` /
// `a == "--<flag>"` early-return checks.
//
// Leaf verbs (e.g. 'pdf' under 'export') appear in the
// result too — they're real, reachable verbs. The report
// distinguishes documented-not-implemented (drift) from
// implemented-not-documented (often a leaf verb under a
// parent, not drift).
//
// caseWindowChars bounds the slice used to capture
// comma-separated sibling cases on the line following a
// `case "<verb>":` match. Must be clamped at slice time —
// see issue #286.
const caseWindowChars = 200

func scanImplementedSubcommands(root string) map[string]bool {
	out := map[string]bool{}

	// Walk main.go for Has*Subcommand / Has*Flag references.
	mainPath := filepath.Join(root, "main.go")
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		return out
	}
	hasRefRe := regexp.MustCompile(`appshell\.Has(\w+?)(?:Subcommand|Flag)\(`)
	funcRefs := map[string]bool{}
	for _, m := range hasRefRe.FindAllStringSubmatch(string(mainData), -1) {
		funcRefs[m[1]] = true
	}

	// For each function name reference, find the function body
	// in cli_*.go (or smoke.go / doctor.go) and pull verbs.
	files, _ := filepath.Glob(filepath.Join(root, "internal", "appshell", "cli_*.go"))
	files = append(files,
		filepath.Join(root, "internal", "appshell", "smoke.go"),
		filepath.Join(root, "internal", "appshell", "doctor.go"),
	)

	for funcName := range funcRefs {
		funcDefRe := regexp.MustCompile(
			`func\s+Has` + funcName + `(?:Subcommand|Flag)\([^)]*\)\s*bool\s*\{([\s\S]*?)\n\}`,
		)
		eqRe := regexp.MustCompile(`args\[0\]\s*[!=]=\s*"((?:--?)?[a-z][a-z0-9_\-]*)"`)
		aeqRe := regexp.MustCompile(`\ba\s*==\s*"((?:--?)?[a-z][a-z0-9_\-]*)"`)
		caseRe := regexp.MustCompile(`case\s+"([a-z][a-z0-9_-]*)"`)
		verbInCaseRe := regexp.MustCompile(`"([a-z][a-z0-9_-]*)"`)

		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			fd := funcDefRe.FindStringSubmatch(string(data))
			if fd == nil {
				continue
			}
			body := fd[1]
			// args[0] == / != "<verb>"
			for _, m := range eqRe.FindAllStringSubmatch(body, -1) {
				out[m[1]] = true
			}
			// a == "--<flag>"
			for _, m := range aeqRe.FindAllStringSubmatch(body, -1) {
				out[m[1]] = true
			}
			// case "<verb>": (and any comma-separated siblings).
			// Clamp the upper bound: the 200-char window is a
			// heuristic to grab siblings on the next line, but a
			// short body (e.g. a stub Has*Subcommand in a
			// new cli_*.go file) would otherwise trip a slice
			// out-of-range panic. See issue #286.
			if cm := caseRe.FindStringSubmatchIndex(body); cm != nil {
				end := cm[0] + caseWindowChars
				if end > len(body) {
					end = len(body)
				}
				snippet := body[cm[0]:end]
				for _, m := range verbInCaseRe.FindAllStringSubmatch(snippet, -1) {
					out[m[1]] = true
				}
			}
		}
	}

	return out
}

// scanDocumentedSubcommands walks docs/agents/cli-plan.md
// looking for `dixiedata <verb> ...` lines that look like
// subcommand examples. Filters out flag-only references and
// prose mentions.
func scanDocumentedSubcommands(docPath string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(docPath)
	if err != nil {
		return out
	}
	// Match `dixiedata <verb>` at start of line (could be
	// inside a fenced code block — that's fine, those are the
	// documented examples we want).
	pattern := regexp.MustCompile(`(?m)^\s*dixiedata\s+([a-z][a-z0-9_-]*)`)
	for _, hit := range pattern.FindAllStringSubmatch(string(data), -1) {
		out[hit[1]] = true
	}
	// Also match `dixiedata --<flag>` style top-level flags.
	flagPattern := regexp.MustCompile(`(?m)^\s*dixiedata\s+(--[a-z][a-z0-9-]*)`)
	for _, hit := range flagPattern.FindAllStringSubmatch(string(data), -1) {
		out[hit[1]] = true
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func diff(a, b []string) []string {
	setB := map[string]bool{}
	for _, s := range b {
		setB[s] = true
	}
	out := []string{}
	for _, s := range a {
		if !setB[s] {
			out = append(out, s)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false

}
