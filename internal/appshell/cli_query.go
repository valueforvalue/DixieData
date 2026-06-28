// cli_query.go — read-only Phase 3 subcommands: `list`,
// `show`, `search`. Dispatch to existing `*App.soldiers`
// facade methods; no new business logic. Useful for scripting
// research workflows without the GUI.
//
//   dixiedata list soldiers [--query Q] [--limit N] [--page P] [--json]
//   dixiedata show soldier <id|display-id> [--json]
//   dixiedata search <query> [--limit N] [--page P] [--json]
//
// `--json` switches output to a stable JSON envelope; default is
// a human-readable fixed-width table. Both modes share the same
// underlying data path — only the rendering differs.
package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/valueforvalue/DixieData/internal/models"
)

// QueryCommand identifies which Phase 3 subcommand was
// requested. Dispatch in RunQuery picks the handler.
type QueryCommand int

const (
	QueryUnknown QueryCommand = iota
	QueryListSoldiers
	QueryShowSoldier
	QuerySearchSoldiers
)

// QueryOptions configures RunQuery. Zero value = text output,
// default page (1) and limit (50).
type QueryOptions struct {
	Command  QueryCommand
	Args     []string // positional args after the subcommand
	Query    string   // --query value
	Limit    int      // --limit value (0 = use default)
	Page     int      // --page value (0 = use default)
	JSON     bool     // --json
	Writer   io.Writer
	App      *App
	Now      func() int64 // for testable timestamps
}

// queryDefaults returns the default limit/page when the user
// didn't specify them. Centralised so list and search stay in
// sync.
func queryDefaults() (limit, page int) { return 50, 1 }

// RunQuery dispatches to the right handler based on
// opts.Command. Returns exit code (0 success, 1 not-found,
// 3 usage error, 2 environment error).
func RunQuery(ctx context.Context, opts QueryOptions) (int, error) {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	defaultLimit, defaultPage := queryDefaults()
	if opts.Limit <= 0 {
		opts.Limit = defaultLimit
	}
	if opts.Page <= 0 {
		opts.Page = defaultPage
	}

	app := opts.App
	if app == nil {
		return 2, fmt.Errorf("RunQuery requires opts.App (or use RunQuery via main.go which builds one)")
	}
	if app.soldiers == nil {
		return 2, fmt.Errorf("soldiers service not initialized; app.startup() must run first")
	}

	switch opts.Command {
	case QueryListSoldiers:
		return runListSoldiers(ctx, app, opts)
	case QueryShowSoldier:
		return runShowSoldier(ctx, app, opts)
	case QuerySearchSoldiers:
		return runSearchSoldiers(ctx, app, opts)
	default:
		return 3, fmt.Errorf("unknown query command")
	}
}

// --- list soldiers ---

func runListSoldiers(ctx context.Context, app *App, opts QueryOptions) (int, error) {
	page := opts.Page
	pageSize := opts.Limit

	// If --query is set, prefer SearchPage so the result honors
	// the user's intent. Otherwise List() gives a paginated view
	// without forcing a WHERE clause.
	var (
		soldiers []models.Soldier
		total    int
		err      error
	)
	if q := strings.TrimSpace(opts.Query); q != "" {
		soldiers, total, err = app.soldiers.SearchPage(q, page, pageSize)
	} else {
		soldiers, total, err = app.soldiers.List(page, pageSize)
	}
	if err != nil {
		return 2, fmt.Errorf("list soldiers: %w", err)
	}
	renderSoldierTable(opts.Writer, soldiers, total, page, pageSize, opts.JSON)
	return 0, nil
}

// --- show soldier ---

func runShowSoldier(ctx context.Context, app *App, opts QueryOptions) (int, error) {
	if len(opts.Args) == 0 {
		return 3, fmt.Errorf("show soldier requires <id|display-id>")
	}
	target := strings.TrimSpace(opts.Args[0])
	if target == "" {
		return 3, fmt.Errorf("show soldier requires <id|display-id>")
	}

	// If target parses as int64, use GetByID; otherwise treat as
	// display ID. This lets users pass either form without a
	// separate flag.
	var (
		soldier *models.Soldier
		err     error
	)
	if id, idErr := strconv.ParseInt(target, 10, 64); idErr == nil && id > 0 {
		soldier, err = app.soldiers.GetByID(id)
	} else {
		soldier, err = app.soldiers.GetByDisplayID(target)
	}
	if err != nil {
		// GetByID/GetByDisplayID return errors for missing
		// records; surface that as exit 1 so scripts can
		// distinguish not-found from usage-error.
		return 1, fmt.Errorf("show soldier %q: %w", target, err)
	}
	renderSoldierDetail(opts.Writer, soldier, opts.JSON)
	return 0, nil
}

// --- search soldiers ---

func runSearchSoldiers(ctx context.Context, app *App, opts QueryOptions) (int, error) {
	q := strings.TrimSpace(opts.Query)
	if q == "" && len(opts.Args) > 0 {
		q = strings.TrimSpace(opts.Args[0])
	}
	if q == "" {
		return 3, fmt.Errorf("search requires a query string (--query Q or positional arg)")
	}
	soldiers, total, err := app.soldiers.SearchPage(q, opts.Page, opts.Limit)
	if err != nil {
		return 2, fmt.Errorf("search: %w", err)
	}
	renderSoldierTable(opts.Writer, soldiers, total, opts.Page, opts.Limit, opts.JSON)
	return 0, nil
}

// --- rendering ---

// renderSoldierTable writes a fixed-width table to w. Columns
// chosen to fit a standard 120-char terminal; long fields are
// truncated with "…" to keep alignment stable.
func renderSoldierTable(w io.Writer, soldiers []models.Soldier, total, page, pageSize int, asJSON bool) {
	if asJSON {
		writeSoldierTableJSON(w, soldiers, total, page, pageSize)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tDISPLAY-ID\tNAME\tBIRTH\tDEATH\tUNIT\tENTRY")
	for _, s := range soldiers {
		name := formatSoldierName(s)
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.ID,
			truncate(s.DisplayID, 14),
			truncate(name, 32),
			truncate(s.BirthDate, 10),
			truncate(s.DeathDate, 10),
			truncate(s.Unit, 24),
			truncate(s.EntryType, 12),
		)
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%d result(s); page %d of %d (showing up to %d per page)\n",
		total, page, pageCount(total, pageSize), pageSize)
}

func pageCount(total, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	if pages == 0 {
		return 1
	}
	return pages
}

// formatSoldierName assembles a display name from prefix +
// first + middle + last + suffix. Missing parts are skipped so
// we don't end up with "  John  " for soldiers that only have a
// first name.
func formatSoldierName(s models.Soldier) string {
	var parts []string
	if s.ShowPrefixBeforeName {
		if p := strings.TrimSpace(s.Prefix); p != "" {
			parts = append(parts, p)
		}
	}
	if fn := strings.TrimSpace(s.FirstName); fn != "" {
		parts = append(parts, fn)
	}
	if mn := strings.TrimSpace(s.MiddleName); mn != "" {
		parts = append(parts, mn)
	}
	if ln := strings.TrimSpace(s.LastName); ln != "" {
		parts = append(parts, ln)
	}
	if !s.ShowPrefixBeforeName {
		if p := strings.TrimSpace(s.Prefix); p != "" {
			parts = append(parts, "("+p+")")
		}
	}
	if sx := strings.TrimSpace(s.Suffix); sx != "" {
		parts = append(parts, sx)
	}
	return strings.Join(parts, " ")
}

// renderSoldierDetail writes a multi-line key/value dump. Uses
// the same JSON envelope as the table command so JSON consumers
// see one consistent shape.
func renderSoldierDetail(w io.Writer, s *models.Soldier, asJSON bool) {
	if asJSON {
		writeSoldierDetailJSON(w, s)
		return
	}
	fmt.Fprintf(w, "ID:              %d\n", s.ID)
	fmt.Fprintf(w, "Display ID:      %s\n", s.DisplayID)
	fmt.Fprintf(w, "Name:            %s\n", formatSoldierName(*s))
	if rank := strings.TrimSpace(s.Rank); rank != "" {
		fmt.Fprintf(w, "Rank:            %s\n", rank)
	}
	if unit := strings.TrimSpace(s.Unit); unit != "" {
		fmt.Fprintf(w, "Unit:            %s\n", unit)
	}
	if bd := strings.TrimSpace(s.BirthDate); bd != "" {
		fmt.Fprintf(w, "Birth:           %s\n", bd)
	}
	if dd := strings.TrimSpace(s.DeathDate); dd != "" {
		fmt.Fprintf(w, "Death:           %s\n", dd)
	}
	if bid := strings.TrimSpace(s.BuriedIn); bid != "" {
		fmt.Fprintf(w, "Buried In:       %s\n", bid)
	}
	if ps := strings.TrimSpace(s.PensionState); ps != "" {
		fmt.Fprintf(w, "Pension State:   %s\n", ps)
	}
	if ch := strings.TrimSpace(s.ConfederateHomeStatus); ch != "" {
		fmt.Fprintf(w, "Confederate Home: %s\n", ch)
	}
	fmt.Fprintf(w, "Entry Type:      %s\n", s.EntryType)
	if s.NeedsReview {
		fmt.Fprintf(w, "Needs Review:    YES (%s)\n", s.ReviewReason)
	}
	if n := strings.TrimSpace(s.Notes); n != "" {
		fmt.Fprintf(w, "\nNotes:\n%s\n", indent(n, "  "))
	}
	if len(s.Records) > 0 {
		fmt.Fprintf(w, "\nSource Records (%d):\n", len(s.Records))
		for _, r := range s.Records {
			line := fmt.Sprintf("  [%s] %s", r.RecordType, r.AppID)
			if d := strings.TrimSpace(r.Details); d != "" {
				line += " — " + truncate(d, 60)
			}
			fmt.Fprintln(w, line)
		}
	}
	if len(s.Images) > 0 {
		fmt.Fprintf(w, "\nImages (%d):\n", len(s.Images))
		for _, img := range s.Images {
			line := fmt.Sprintf("  %s", img.FileName)
			if img.IsPrimary {
				line += " (primary)"
			}
			if c := strings.TrimSpace(img.Caption); c != "" {
				line += " — " + truncate(c, 60)
			}
			fmt.Fprintln(w, line)
		}
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// --- JSON output ---

type soldierEnvelope struct {
	Soldiers []models.Soldier `json:"soldiers"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	Pages    int              `json:"pages"`
	PageSize int              `json:"page_size"`
}

func writeSoldierTableJSON(w io.Writer, soldiers []models.Soldier, total, page, pageSize int) {
	env := soldierEnvelope{
		Soldiers: soldiers,
		Total:    total,
		Page:     page,
		Pages:    pageCount(total, pageSize),
		PageSize: pageSize,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

func writeSoldierDetailJSON(w io.Writer, s *models.Soldier) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s)
}

// --- CLI arg parsing ---

// ParseQueryCommand inspects os.Args and returns the
// QueryCommand + parsed QueryOptions. Returns QueryUnknown if
// args don't start with one of our subcommand names.
//
// Parsing is deliberately minimal — no cobra, no kingpin. We
// walk the slice once and pick out:
//   - first arg: subcommand verb ("list" / "show" / "search")
//   - second arg for `show`: target (id or display-id)
//   - second arg for `search`: query (optional, --query also works)
//   - any --flag tokens for the rest
func ParseQueryCommand(args []string) (QueryOptions, error) {
	opts := QueryOptions{}
	if len(args) == 0 {
		return opts, nil
	}
	switch args[0] {
	case "list":
		opts.Command = QueryListSoldiers
		// next arg may be the entity ("soldiers"); we ignore it
		// because `list` only supports soldiers for now.
		if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
			_ = args[1]
		}
	case "show":
		opts.Command = QueryShowSoldier
		// Show's target is always the LAST non-flag positional
		// arg, no matter where flags are interleaved:
		//   show soldier 54
		//   show soldier DXD-00052 --json
		//   show soldier --json DXD-00052
		// All resolve to target=DXD-00052.
		for i := len(args) - 1; i >= 1; i-- {
			if strings.HasPrefix(args[i], "--") {
				continue
			}
			opts.Args = append(opts.Args, args[i])
			break
		}
	case "search":
		opts.Command = QuerySearchSoldiers
		// search takes a verb + query string:
		//   search "Smith"
		//   search --query "Smith"
		if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
			opts.Args = append(opts.Args, args[1])
		}
	default:
		return opts, nil // not a query subcommand; let caller ignore
	}

	for i, a := range args {
		switch {
		case a == "--json":
			opts.JSON = true
		case strings.HasPrefix(a, "--query="):
			opts.Query = strings.TrimPrefix(a, "--query=")
		case a == "--query" && i+1 < len(args):
			opts.Query = args[i+1]
		case strings.HasPrefix(a, "--limit="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--limit=")); err == nil {
				opts.Limit = n
			}
		case a == "--limit" && i+1 < len(args):
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				opts.Limit = n
			}
		case strings.HasPrefix(a, "--page="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--page=")); err == nil {
				opts.Page = n
			}
		case a == "--page" && i+1 < len(args):
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				opts.Page = n
			}
		}
	}
	return opts, nil
}

// HasQuerySubcommand returns true when the first arg is one of
// our Phase 3 subcommands. main.go uses this to dispatch into
// RunQuery before falling through to wails.Run.
func HasQuerySubcommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "list", "show", "search":
		return true
	}
	return false
}