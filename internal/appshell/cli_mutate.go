// cli_mutate.go -- write subcommands for the CLI (issue #371
// Phase 8). Read-only commands (list / show / search) live in
// cli_query.go; write commands (soldier create / update /
// delete) live here. The CLI dispatches to existing *App
// facade methods; no new business logic.
//
//   dixiedata soldier create --from <path> | --from-stdin [--json]
//
// `--json` switches output to a stable envelope ({"id":N,
// "display_id":"DXD-XXXXX", ...}) so a script can chain via
// `jq -r .id | xargs ... show`. Default is a single human-
// readable line: "created soldier 42 (DXD-00042)".
//
// Destructive verbs (`--from-stdin` is the only one in v1;
// future update/delete also) accept `--dry-run` and `--yes`.
// `--dry-run` parses + validates the input but does not write.
// `--yes` skips the confirm prompt (the prompt only fires
// when stdin is a TTY; in scripts we already trust the
// caller).
package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
)

// MutateCommand identifies which Phase 8 subcommand was
// requested. Dispatch in RunMutate picks the handler.
type MutateCommand int

const (
	MutateUnknown MutateCommand = iota
	MutateSoldierCreate
	MutateSoldierUpdate
	MutateSoldierDelete
)

// MutateOptions configures RunMutate. Zero value = text output.
type MutateOptions struct {
	Command   MutateCommand
	From      string // --from <path> input file
	FromStdin bool   // --from-stdin input
	TargetID  string // <id|dxd-id> positional for update/delete
	JSON      bool   // --json envelope output
	DryRun    bool   // --dry-run
	Yes       bool   // --yes (skip confirm)
	Writer    io.Writer
	App       *App
}

// HasMutateSubcommand returns true when the args start with a
// known Phase 8 mutate verb. main.go uses this to dispatch
// into RunMutate before falling through to wails.Run.
//
// Recognised: `soldier create`. Update / delete / event / tag
// verbs land in follow-up slices per the tracer-bullet rule.
func HasMutateSubcommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	if args[0] != "soldier" {
		return false
	}
	switch args[1] {
	case "create", "update", "delete":
		return true
	}
	return false
}

// ParseMutateCommand parses the CLI args for a mutate
// subcommand. Returns (opts, true) on success, (zero, false)
// when the args don't match a known subcommand. The caller
// (main.go dispatcher) checks ok before calling RunMutate.
func ParseMutateCommand(args []string) (MutateOptions, bool) {
	// Find the subcommand name (first arg).
	if len(args) == 0 {
		return MutateOptions{}, false
	}
	sub := args[0]
	rest := args[1:]
	// Skip the global flags (--data-dir, --json, etc.) so
	// the subcommand parser sees a clean argv.
	filtered := make([]string, 0, len(rest))
	jsonSeen := false
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch a {
		case "--json":
			jsonSeen = true
		case "--data-dir", "--db":
			i++ // skip value
		default:
			filtered = append(filtered, a)
		}
	}

	switch sub {
	case "soldier":
		if len(filtered) == 0 {
			return MutateOptions{}, false
		}
		verb := filtered[0]
		switch verb {
		case "create":
			opts := MutateOptions{Command: MutateSoldierCreate, JSON: jsonSeen}
			// Parse create-specific flags from filtered[1:].
			// --from-stdin is a bare flag (no value), so it
			// needs a separate loop from --from (which
			// consumes the next arg).
			for _, a := range filtered[1:] {
				switch a {
				case "--from-stdin":
					opts.FromStdin = true
				case "--dry-run":
					opts.DryRun = true
				case "--yes":
					opts.Yes = true
				}
			}
			for i := 0; i < len(filtered)-1; i++ {
				if filtered[i] == "--from" {
					opts.From = filtered[i+1]
				}
			}
			if opts.From == "" && !opts.FromStdin {
				return MutateOptions{}, false
			}
			return opts, true
		case "update":
			// `soldier update <id|dxd-id> --from <path> | --from-stdin`
			// The id is the first positional after `update`;
			// the same --from/--from-stdin flags as create.
			// Must be a non-flag arg, otherwise the caller
			// forgot the id.
			if len(filtered) < 2 || strings.HasPrefix(filtered[1], "--") {
				return MutateOptions{}, false
			}
			opts := MutateOptions{
				Command:  MutateSoldierUpdate,
				TargetID: filtered[1],
				JSON:     jsonSeen,
			}
			for _, a := range filtered[2:] {
				switch a {
				case "--from-stdin":
					opts.FromStdin = true
				case "--dry-run":
					opts.DryRun = true
				case "--yes":
					opts.Yes = true
				}
			}
			for i := 0; i < len(filtered)-1; i++ {
				if filtered[i] == "--from" {
					opts.From = filtered[i+1]
				}
			}
			if opts.From == "" && !opts.FromStdin {
				return MutateOptions{}, false
			}
			return opts, true
		case "delete":
			// `soldier delete <id|dxd-id> [--yes]`
			// No --from; the row id is the only input.
			// --yes is a future hook for the TTY confirm
			// prompt (deferred — no prompt in v1 because
			// no one runs this by hand yet).
			if len(filtered) < 2 || strings.HasPrefix(filtered[1], "--") {
				return MutateOptions{}, false
			}
			opts := MutateOptions{
				Command:  MutateSoldierDelete,
				TargetID: filtered[1],
				JSON:     jsonSeen,
			}
			for _, a := range filtered[2:] {
				if a == "--yes" {
					opts.Yes = true
				}
			}
			return opts, true
		}
	}
	return MutateOptions{}, false
}

// RunMutate dispatches to the right handler based on
// opts.Command. Returns exit code (0 success, 1 op-failed,
// 2 env-error, 3 usage). Errors are written to opts.Writer
// for human output and to the result for JSON output.
func RunMutate(ctx context.Context, opts MutateOptions) (int, error) {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	app := opts.App
	if app == nil {
		return 2, fmt.Errorf("RunMutate requires opts.App (or use RunMutate via main.go which builds one)")
	}
	if app.soldiers == nil {
		return 2, fmt.Errorf("soldiers service not initialized; app.startup() must run first")
	}

	switch opts.Command {
	case MutateSoldierCreate:
		return runMutateSoldierCreate(ctx, app, opts)
	case MutateSoldierUpdate:
		return runMutateSoldierUpdate(ctx, app, opts)
	case MutateSoldierDelete:
		return runMutateSoldierDelete(ctx, app, opts)
	default:
		return 3, fmt.Errorf("unknown mutate command")
	}
}

func runMutateSoldierCreate(ctx context.Context, app *App, opts MutateOptions) (int, error) {
	// Read the input.
	var input []byte
	var err error
	if opts.FromStdin {
		input, err = io.ReadAll(os.Stdin)
	} else {
		input, err = os.ReadFile(opts.From)
	}
	if err != nil {
		return 1, fmt.Errorf("read input: %w", err)
	}
	var soldier models.Soldier
	if err := json.Unmarshal(input, &soldier); err != nil {
		return 1, fmt.Errorf("parse input JSON: %w", err)
	}
	if opts.DryRun {
		// Parse-only check: if JSON unmarshals, the input is
		// at least syntactically valid. Real dry-run validation
		// (required fields, FK existence) lives in the service
		// layer; we surface a 0-exit with a parsed preview for
		// the script to chain.
		if opts.JSON {
			fmt.Fprintln(opts.Writer, `{"dry_run":true,"ok":true}`)
		} else {
			fmt.Fprintf(opts.Writer, "dry-run: input parses cleanly (%d bytes)\n", len(input))
		}
		return 0, nil
	}
	created, err := app.soldiers.Create(soldier)
	if err != nil {
		return 1, fmt.Errorf("create soldier: %w", err)
	}
	if opts.JSON {
		// Locked decision (4) from #371: echo the new row's
		// canonical state so scripts can chain.
		// "id" + "display_id" are the two fields a script
		// realistically needs; the full soldier struct is
		// available via `show soldier <id>` for the rest.
		out := map[string]any{
			"id":         created.ID,
			"display_id": created.DisplayID,
		}
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "created soldier %d (%s)\n", created.ID, created.DisplayID)
	return 0, nil
}

// runMutateSoldierUpdate reads a models.Soldier JSON from
// --from/--from-stdin, resolves the positional <id|dxd-id> to a
// row id, and dispatches to app.soldiers.Update. Mirrors the
// create handler's read/parse/dispatch shape; the id resolution
// lives in resolveSoldierTargetID so the same code path serves
// both update and delete.
func runMutateSoldierUpdate(ctx context.Context, app *App, opts MutateOptions) (int, error) {
	id, err := resolveSoldierTargetID(ctx, app, opts.TargetID)
	if err != nil {
		return 1, fmt.Errorf("resolve target id: %w", err)
	}
	var input []byte
	if opts.FromStdin {
		input, err = io.ReadAll(os.Stdin)
	} else {
		input, err = os.ReadFile(opts.From)
	}
	if err != nil {
		return 1, fmt.Errorf("read input: %w", err)
	}
	var soldier models.Soldier
	if err := json.Unmarshal(input, &soldier); err != nil {
		return 1, fmt.Errorf("parse input JSON: %w", err)
	}
	// The positional id is the source of truth; the input
	// JSON's ID is overwritten so a script that forgot to set
	// it (or set the wrong one) can't accidentally re-key
	// the row.
	soldier.ID = id
	if opts.DryRun {
		if opts.JSON {
			fmt.Fprintln(opts.Writer, `{"dry_run":true,"ok":true}`)
		} else {
			fmt.Fprintf(opts.Writer, "dry-run: input parses cleanly (%d bytes); would update id=%d\n", len(input), id)
		}
		return 0, nil
	}
	if err := app.soldiers.Update(soldier); err != nil {
		return 1, fmt.Errorf("update soldier: %w", err)
	}
	// Re-fetch the row so the JSON echo carries the
	// post-update DisplayID (the Update() path mutates the
	// row in place and the caller's struct may not reflect
	// the new normalization).
	got, err := app.soldiers.GetByID(id)
	if err != nil {
		return 1, fmt.Errorf("re-read soldier: %w", err)
	}
	if opts.JSON {
		out := map[string]any{
			"id":         got.ID,
			"display_id": got.DisplayID,
		}
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "updated soldier %d (%s)\n", got.ID, got.DisplayID)
	return 0, nil
}

// runMutateSoldierDelete resolves the positional <id|dxd-id> to a
// row id and dispatches to app.soldiers.Delete. No --from input
// in v1; the id is the only argument. --yes is accepted but
// currently a no-op (no TTY confirm prompt in v1 — scripts are
// the primary caller).
func runMutateSoldierDelete(ctx context.Context, app *App, opts MutateOptions) (int, error) {
	id, err := resolveSoldierTargetID(ctx, app, opts.TargetID)
	if err != nil {
		return 1, fmt.Errorf("resolve target id: %w", err)
	}
	if err := app.soldiers.Delete(id); err != nil {
		return 1, fmt.Errorf("delete soldier: %w", err)
	}
	if opts.JSON {
		out := map[string]any{
			"id":  id,
			"ok":  true,
		}
		enc := json.NewEncoder(opts.Writer)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "deleted soldier %d\n", id)
	return 0, nil
}

// resolveSoldierTargetID accepts either a numeric id (e.g. "42")
// or a DisplayID (e.g. "DXD-00001") and returns the numeric row
// id. Shared by update + delete; lives in cli_mutate.go because
// the resolution rule is CLI-shape (positional argv, not a form
// value), and the soldier_service has its own GetByDisplayID that
// the GUI uses for its own purposes.
func resolveSoldierTargetID(ctx context.Context, app *App, target string) (int64, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0, fmt.Errorf("target id is required")
	}
	if id, err := strconv.ParseInt(target, 10, 64); err == nil {
		return id, nil
	}
	// Not numeric — treat as a DisplayID.
	s, err := app.soldiers.GetByDisplayID(target)
	if err != nil {
		return 0, fmt.Errorf("lookup by display_id %q: %w", target, err)
	}
	return s.ID, nil
}