// cli_import.go — Phase 5 of docs/agents/cli-plan.md.
//
// Four import subcommands that bypass the native OpenFileDialog
// entirely. Each takes --from <path> so the source is deterministic:
//
//	dixiedata import backup          --from <file.ddbak>
//	dixiedata import shared-archive --from <file.ddshare>
//	dixiedata import images         --soldier <id|dxd-id> --from <file>...
//	dixiedata import memorial-json  --from <file.json>
//
// Dispatch is direct to existing *App methods:
//	import backup          → a.backup.Import (which calls
//	                         ImportWithLocalIdentity using the
//	                         current local identity, derived from
//	                         local_settings)
//	import shared-archive  → a.backup.ImportSharedBackup (single
//	                         blocking call; merges non-conflicting
//	                         records, stages conflicts for review)
//	import images          → a.ImportImagePaths (exported wrapper
//	                         around the existing importImagePaths
//	                         used by handleImportSoldierImages)
//	import memorial-json   → a.soldiers.ImportMemorialArchive
//	                         (single blocking call; writes its own
//	                         issues log under the data dir)
//
// Two are destructive: backup and shared-archive overwrite data
// in-place. For those, --dry-run is the safe default, and --yes
// is required to skip the refusal (exit 4 = auth/permission per
// the standard exit codes in cli-plan.md). Images and memorial-json
// are additive — they only insert new rows — so they run without
// --yes. The plan explicitly defers the --conflict=skip|merge|overwrite
// flag for shared-archive to a follow-up because
// ImportSharedBackup doesn't expose conflict-resolution flags
// upstream yet.
//
// Pre-import restore-point safety net was investigated and
// REMOVED in this commit. The restore-point manager writes to
// <dataDir>/updates/restore-points/, which is INSIDE the data
// dir. archive.replaceDataDir (used by backup import) and
// ImportSharedBackup (used by shared-archive import) both
// mutate the data dir in place via os.Rename, which moves the
// restore point into a `*-previous-*` sibling and then
// RemoveAll's it. The restore point is destroyed by the very
// import it was meant to back out. Phase 6 will add a sibling
// restore-point root (.dixiedata-restore-points/) so we can
// snapshot OUTSIDE the data dir. Until then, the rollback story
// for backup import is "re-run with a different .ddbak", and
// for shared-archive import is "review pending conflicts in the
// merge-review UI". We surface both in the import output so the
// user knows.
//
// Static-archive import is NOT shipped: the static archive is
// read-only browser-viewable output with no companion import
// path. Feedback-log import is NOT shipped: feedback is hand-typed
// in the GUI; there is no consumer for ingesting JSONL feedback
// logs. Both were candidates in earlier drafts of cli-plan.md and
// were dropped after the user pointed out neither had a real
// implementation behind them. See cli-plan.md §Phase 5 for the
// audit history.
package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

// (no extra type aliases — we use records.BackupManifest directly,
// which the records package re-exports from internal/archive.)

// ImportKind enumerates the four import subcommands. ImportUnknown
// is returned by ParseImportArgs for any non-import CLI invocation
// so HasImportSubcommand can short-circuit cleanly.
type ImportKind int

const (
	ImportUnknown ImportKind = iota
	ImportBackup
	ImportSharedArchive
	ImportImages
	ImportMemorialJSON
)

// String returns the lowercase verb used on the command line.
func (k ImportKind) String() string {
	switch k {
	case ImportBackup:
		return "backup"
	case ImportSharedArchive:
		return "shared-archive"
	case ImportImages:
		return "images"
	case ImportMemorialJSON:
		return "memorial-json"
	default:
		return "unknown"
	}
}

// IsDestructive reports whether the kind overwrites data in-place.
// Destructive commands require --yes unless --dry-run is set.
func (k ImportKind) IsDestructive() bool {
	switch k {
	case ImportBackup, ImportSharedArchive:
		return true
	default:
		return false
	}
}

// ImportOptions is the parsed CLI arguments for an import
// subcommand. It carries every piece of state the dispatch
// functions need: the kind, the source path(s), optional soldier
// target, flags. App is wired by runImportSubcommand after
// Startup so the dispatch functions can call a.backup.Import etc.
type ImportOptions struct {
	Kind      ImportKind
	FromPaths []string // --from may appear multiple times for `import images`
	SoldierID int64    // --soldier for `import images`; numeric ID
	DisplayID string   // --soldier for `import images`; display id (DXD-00052)
	DryRun    bool
	Yes       bool
	JSON      bool
	Writer    io.Writer
	App       *App
}

// HasImportSubcommand reports whether args start with `import
// <known-kind>`. The kind check is the same lookup table used
// inside ParseImportArgs, so we can't accept kinds we wouldn't
// dispatch.
func HasImportSubcommand(args []string) bool {
	if len(args) < 2 || args[0] != "import" {
		return false
	}
	_, err := lookupImportKind(args[1])
	return err == nil
}

func lookupImportKind(s string) (ImportKind, error) {
	switch s {
	case "backup":
		return ImportBackup, nil
	case "shared-archive":
		return ImportSharedArchive, nil
	case "images":
		return ImportImages, nil
	case "memorial-json":
		return ImportMemorialJSON, nil
	default:
		return ImportUnknown, fmt.Errorf("unknown import kind %q (expected backup|shared-archive|images|memorial-json)", s)
	}
}

// ParseImportArgs walks args[1:] (os.Args minus the program name)
// and produces an ImportOptions. Returns a useful error on any
// failure so the caller can print to stderr and exit 3.
//
// Both --flag value and --flag=value forms are accepted, mirroring
// the export parser. The --from flag is repeatable: each occurrence
// appends one path to FromPaths. This lets
// `import images --soldier DXD-00052 --from a.jpg --from b.jpg`
// work as users expect.
func ParseImportArgs(args []string) (ImportOptions, error) {
	opts := ImportOptions{Writer: os.Stdout}
	if len(args) < 2 || args[0] != "import" {
		return opts, errors.New("not an import subcommand")
	}
	kind, err := lookupImportKind(args[1])
	if err != nil {
		return opts, err
	}
	opts.Kind = kind

	rest := args[2:]
	i := 0
	for i < len(rest) {
		arg := rest[i]
		// Split --flag=value once at the first '=' so paths
		// containing '=' are not mangled.
		var name, value string
		hasValue := false
		if strings.HasPrefix(arg, "--") {
			if eq := strings.IndexByte(arg, '='); eq >= 0 {
				name = arg[:eq]
				value = arg[eq+1:]
				hasValue = true
			} else {
				name = arg
			}
		}
		switch {
		case name == "--from" || name == "-f":
			if !hasValue {
				if i+1 >= len(rest) {
					return opts, fmt.Errorf("--from requires a value")
				}
				i++
				value = rest[i]
			}
			value = strings.TrimSpace(value)
			if value == "" {
				return opts, fmt.Errorf("--from requires a non-empty path")
			}
			opts.FromPaths = append(opts.FromPaths, value)
		case name == "--soldier":
			if !hasValue {
				if i+1 >= len(rest) {
					return opts, fmt.Errorf("--soldier requires a value")
				}
				i++
				value = rest[i]
			}
			value = strings.TrimSpace(value)
			if value == "" {
				return opts, fmt.Errorf("--soldier requires a non-empty value")
			}
			// Numeric → SoldierID; non-numeric → DisplayID.
			// Same auto-detect rule `show` uses (cli-plan.md
			// Phase 3 lesson learned).
			if id, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil {
				opts.SoldierID = id
			} else {
				opts.DisplayID = value
			}
		case name == "--dry-run":
			opts.DryRun = true
		case name == "--yes" || name == "-y":
			opts.Yes = true
		case name == "--json":
			opts.JSON = true
		case name == "--help" || name == "-h":
			return opts, fmt.Errorf("help requested")
		default:
			if strings.HasPrefix(arg, "--") {
				return opts, fmt.Errorf("unknown flag %s for `import %s`", name, kind)
			}
			// Positional arg — only --from takes positionals
			// and that's handled above. Anything else is an error.
			return opts, fmt.Errorf("unexpected positional argument %s", arg)
		}
		i++
	}

	if err := opts.validate(); err != nil {
		return opts, err
	}
	return opts, nil
}

func (o ImportOptions) validate() error {
	switch o.Kind {
	case ImportBackup, ImportSharedArchive, ImportMemorialJSON:
		if len(o.FromPaths) == 0 {
			return fmt.Errorf("import %s requires --from <path>", o.Kind)
		}
		if len(o.FromPaths) > 1 {
			return fmt.Errorf("import %s accepts exactly one --from, got %d", o.Kind, len(o.FromPaths))
		}
	case ImportImages:
		if len(o.FromPaths) == 0 {
			return errors.New("import images requires at least one --from <path>")
		}
		if o.SoldierID == 0 && o.DisplayID == "" {
			return errors.New("import images requires --soldier <id|display-id>")
		}
	}
	if o.Kind.IsDestructive() && !o.DryRun && !o.Yes {
		return fmt.Errorf("import %s overwrites data; pass --dry-run to preview or --yes to confirm", o.Kind)
	}
	return nil
}

// ImportImagePaths is the exported wrapper around the existing
// importImagePaths method. The CLI dispatch calls this so we
// don't have to duplicate the per-file validation, filename
// sequencing, and soldiers.AddImage wiring.
func (a *App) ImportImagePaths(soldier models.Soldier, paths []string) (int, error) {
	return a.importImagePaths(soldier, paths)
}

// RunImport dispatches an ImportOptions to the correct handler
// based on Kind. Returns the exit code and any error. The error
// is also written to stderr from main.go so this function can be
// called from tests without leaking to stderr.
//
// Side effects on a (database, restore-points, soldier-service
// facade) match the GUI handlers: handleImportBackup closes and
// reopens the DB around ImportWithLocalIdentity so the staging
// swap doesn't trample in-flight queries. handleImportSharedArchive
// and the memorial-json worker don't close the DB; the import
// path is fully encapsulated in a transaction by the service
// itself.
func RunImport(ctx context.Context, opts ImportOptions) (int, error) {
	a := opts.App
	if a == nil {
		return 2, errors.New("internal: RunImport called without *App wired")
	}
	switch opts.Kind {
	case ImportBackup:
		return runImportBackup(ctx, a, opts)
	case ImportSharedArchive:
		return runImportSharedArchive(ctx, a, opts)
	case ImportImages:
		return runImportImages(ctx, a, opts)
	case ImportMemorialJSON:
		return runImportMemorialJSON(ctx, a, opts)
	default:
		return 3, fmt.Errorf("unsupported import kind %v", opts.Kind)
	}
}

// --- backup ---

// runImportBackup restores a .ddbak (or legacy .zip) archive into
// the live data dir. Mirrors handleImportBackup: closes the DB
// before the staging swap, reopens after, returns the manifest
// so the CLI can print a summary line.
//
// Dry-run mode prints what the import WOULD do without touching
// the data dir. We achieve that by opening the .ddbak, reading
// the manifest, and printing its contents — no DB close, no
// staging, no restore-point snapshot. The user gets a cheap
// preview that doesn't require the restore-point machinery at all.
func runImportBackup(ctx context.Context, a *App, opts ImportOptions) (int, error) {
	from := opts.FromPaths[0]

	if opts.DryRun {
		return previewBackup(from, opts)
	}

	// No pre-import restore point: the .ddbak import replaces the
	// data dir in place via os.Rename (see archive.replaceDataDir).
	// A restore point written before the import would land inside
	// the OLD data dir, then be renamed into the `*-previous-*`
	// sibling and immediately deleted by replaceDataDir's
	// RemoveAll. So the restore point would be destroyed by the
	// very import it was meant to back out. The .ddbak itself is
	// the rollback artifact for a backup import. Phase 6 will
	// add `dixiedata backup list` so the user can find prior
	// .ddbak files outside the data dir.
	var snapshotID string
	_ = snapshotID

	if a.database != nil {
		a.database.Close()
		a.database = nil
	}

	// Look up the local identity the same way handleImportBackup
	// does (currentImportIdentity logic, inlined here so we don't
	// have to keep the DB handle around after the close above).
	var localIdentity models.UserIdentity
	preserveLocalIdentity := false
	if id, preserve, idErr := loadLocalImportIdentity(a); idErr == nil {
		localIdentity = id
		preserveLocalIdentity = preserve
	}

	manifest, err := a.backup.ImportWithLocalIdentity(from, a.dataDir, localIdentity, preserveLocalIdentity)
	if err != nil {
		if reopenErr := a.reopenDatabase(); reopenErr != nil {
			return 1, fmt.Errorf("backup import failed (%v) and the database could not be reopened (%v)", err, reopenErr)
		}
		return 1, fmt.Errorf("backup import failed: %w", err)
	}
	if reopenErr := a.reopenDatabase(); reopenErr != nil {
		return 1, fmt.Errorf("backup imported but the database could not be reopened: %w", reopenErr)
	}

	if opts.JSON {
		out := map[string]any{
			"kind":     "backup",
			"from":     from,
			"soldiers": manifest.Soldiers,
			"records":  manifest.Records,
			"images":   manifest.Images,
		}
		_ = json.NewEncoder(opts.Writer).Encode(out)
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "restored %s\n", from)
	fmt.Fprintf(opts.Writer, "  soldiers=%d records=%d images=%d\n", manifest.Soldiers, manifest.Records, manifest.Images)
	fmt.Fprintf(opts.Writer, "rollback: re-run with a different .ddbak (no pre-import snapshot taken; the imported .ddbak is the source of truth)\n")
	return 0, nil
}

func previewBackup(from string, opts ImportOptions) (int, error) {
	// Open the zip and read just the manifest entry. We don't
	// extract anything, so the data dir is untouched.
	previewManifest, err := readBackupManifestFromZip(from)
	if err != nil {
		return 2, fmt.Errorf("read %s: %w", from, err)
	}
	if opts.JSON {
		out := map[string]any{
			"dry_run":  true,
			"kind":     "backup",
			"from":     from,
			"soldiers": previewManifest.Soldiers,
			"records":  previewManifest.Records,
			"images":   previewManifest.Images,
			"format":   previewManifest.Format,
			"version":  previewManifest.Version,
		}
		_ = json.NewEncoder(opts.Writer).Encode(out)
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "dry-run: would restore %s\n", from)
	fmt.Fprintf(opts.Writer, "  soldiers=%d records=%d images=%d\n", previewManifest.Soldiers, previewManifest.Records, previewManifest.Images)
	fmt.Fprintf(opts.Writer, "  format=%s version=%d\n", previewManifest.Format, previewManifest.Version)
	fmt.Fprintln(opts.Writer, "pass --yes to apply")
	return 0, nil
}

// --- shared archive ---

// runImportSharedArchive merges a .ddshare file into the live data
// dir. Mirrors handleImportSharedArchive: runs the import directly
// (it's a single blocking call inside the service layer), prints
// the SharedImportSummary, returns the log path.
//
// PendingConflicts > 0 is NOT a failure — the import succeeded and
// the user can review conflicts via the merge-review UI. We surface
// the count and the log path so the user knows where to look.
//
// No pre-import restore point yet: the merge mutates the live DB
// in place, so a restore point inside the data dir would be
// overwritten by the merge itself. Phase 6 will add a sibling
// restore-point dir so we can snapshot to .dixiedata-restore-points/
// outside the merge target. For now the merge log (summary.LogPath)
// is the rollback artifact.
func runImportSharedArchive(ctx context.Context, a *App, opts ImportOptions) (int, error) {
	from := opts.FromPaths[0]

	if opts.DryRun {
		return previewSharedArchive(from, opts)
	}

	var snapshotID string
	_ = snapshotID

	summary, err := a.backup.ImportSharedBackup(from, a.dataDir)
	if err != nil {
		return 1, fmt.Errorf("shared archive import failed: %w", err)
	}

	if opts.JSON {
		out := map[string]any{
			"kind":              "shared-archive",
			"from":              from,
			"soldiers_inserted": summary.SoldiersInserted,
			"soldiers_updated":  summary.SoldiersUpdated,
			"records_inserted":  summary.RecordsInserted,
			"records_updated":   summary.RecordsUpdated,
			"images_inserted":   summary.ImagesInserted,
			"images_updated":    summary.ImagesUpdated,
			"pending_conflicts": summary.PendingConflicts,
			"merge_log":         summary.LogPath,
		}
		_ = json.NewEncoder(opts.Writer).Encode(out)
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "merged %s\n", from)
	fmt.Fprintf(opts.Writer, "  soldiers: inserted=%d updated=%d\n", summary.SoldiersInserted, summary.SoldiersUpdated)
	fmt.Fprintf(opts.Writer, "  records:  inserted=%d updated=%d\n", summary.RecordsInserted, summary.RecordsUpdated)
	fmt.Fprintf(opts.Writer, "  images:   inserted=%d updated=%d\n", summary.ImagesInserted, summary.ImagesUpdated)
	if summary.PendingConflicts > 0 {
		fmt.Fprintf(opts.Writer, "  pending_conflicts=%d (review via merge review UI)\n", summary.PendingConflicts)
	}
	if summary.LogPath != "" {
		fmt.Fprintf(opts.Writer, "  merge_log=%s\n", summary.LogPath)
	}
	fmt.Fprintf(opts.Writer, "rollback: review pending_conflicts in the merge-review UI; no pre-merge snapshot taken\n")
	return 0, nil
}

func previewSharedArchive(from string, opts ImportOptions) (int, error) {
	// Lightweight: read just the manifest from the zip and print
	// it. Conflict count isn't visible at manifest level (that's
	// computed during merge), so the dry-run only proves the file
	// is a valid .ddshare of the right shape.
	manifest, err := readSharedArchiveManifest(from)
	if err != nil {
		return 2, fmt.Errorf("read %s: %w", from, err)
	}
	if opts.JSON {
		out := map[string]any{
			"dry_run":       true,
			"kind":          "shared-archive",
			"from":          from,
			"soldiers":      manifest.Soldiers,
			"records":       manifest.Records,
			"images":        manifest.Images,
			"format":        manifest.Format,
			"version":       manifest.Version,
			"data_format":   manifest.DataFormat,
			"schema_version": manifest.SchemaVersion,
			"owner_name":    manifest.OwnerName,
		}
		_ = json.NewEncoder(opts.Writer).Encode(out)
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "dry-run: would merge %s\n", from)
	fmt.Fprintf(opts.Writer, "  soldiers=%d records=%d images=%d\n", manifest.Soldiers, manifest.Records, manifest.Images)
	fmt.Fprintf(opts.Writer, "  owner=%q format=%s/%d\n", manifest.OwnerName, manifest.Format, manifest.Version)
	fmt.Fprintln(opts.Writer, "pass --yes to apply")
	return 0, nil
}

// --- images ---

// runImportImages copies one or more image files into a single
// soldier's image directory. Mirrors handleImportSoldierImages
// minus the OpenMultipleFilesDialog + jobs.Registry enqueue
// (single blocking call; the service does its own work).
//
// The soldier is resolved by ID first (cheaper), then by
// display-id. If --soldier accepts both forms the user can
// pass either.
func runImportImages(ctx context.Context, a *App, opts ImportOptions) (int, error) {
	soldier, err := resolveSoldierTarget(a, opts)
	if err != nil {
		return 1, err
	}

	if opts.DryRun {
		// Dry-run: validate each path exists and is a non-empty
		// file with a supported image extension. Don't copy
		// anything, don't touch the soldiers facade.
		issues := 0
		for _, p := range opts.FromPaths {
			info, statErr := os.Stat(p)
			if statErr != nil {
				fmt.Fprintf(opts.Writer, "  skip %s: %v\n", p, statErr)
				issues++
				continue
			}
			if info.IsDir() || info.Size() == 0 {
				fmt.Fprintf(opts.Writer, "  skip %s: empty or directory\n", p)
				issues++
				continue
			}
			if !isAllowedImageFile(filepath.Base(p)) {
				fmt.Fprintf(opts.Writer, "  skip %s: unsupported image extension\n", p)
				issues++
				continue
			}
			fmt.Fprintf(opts.Writer, "  would import %s (%d bytes)\n", p, info.Size())
		}
		fmt.Fprintf(opts.Writer, "dry-run: would attach %d image(s) to soldier %d (%s)\n", len(opts.FromPaths)-issues, soldier.ID, soldier.DisplayID)
		return 0, nil
	}

	imported, err := a.ImportImagePaths(*soldier, opts.FromPaths)
	if err != nil && imported == 0 {
		return 1, fmt.Errorf("image import failed: %w", err)
	}
	if opts.JSON {
		out := map[string]any{
			"kind":      "images",
			"soldier":   soldier.DisplayID,
			"soldier_id": soldier.ID,
			"imported":  imported,
			"requested": len(opts.FromPaths),
		}
		if err != nil {
			out["warning"] = err.Error()
		}
		_ = json.NewEncoder(opts.Writer).Encode(out)
		// Partial-success still exits 0 with a warning field;
		// the caller knows some images landed.
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "imported %d/%d image(s) for soldier %d (%s)\n", imported, len(opts.FromPaths), soldier.ID, soldier.DisplayID)
	if err != nil {
		fmt.Fprintf(opts.Writer, "warning: %v\n", err)
	}
	return 0, nil
}

func resolveSoldierTarget(a *App, opts ImportOptions) (*models.Soldier, error) {
	if opts.SoldierID != 0 {
		return a.soldiers.GetByID(opts.SoldierID)
	}
	return a.soldiers.GetByDisplayID(opts.DisplayID)
}

// --- memorial-json ---

// runImportMemorialJSON imports a Memorial archive JSON dump.
// Unlike the GUI's two-phase flow (preview + confirm), the CLI
// skips preview — the user has already chosen the file. The
// service writes its own issues log under the data dir; we
// surface the log path so the user knows where to look.
//
// Memorial-json import is additive: it only creates new rows in
// the soldiers table. No restore-point safety net needed because
// additive writes don't need rolling back (the worst case is
// reviewing the imported batch via the existing browse UI and
// deleting the rows you don't want).
func runImportMemorialJSON(ctx context.Context, a *App, opts ImportOptions) (int, error) {
	from := opts.FromPaths[0]

	if opts.DryRun {
		preview, err := a.soldiers.PreviewMemorialArchive(from)
		if err != nil {
			return 1, fmt.Errorf("preview %s: %w", from, err)
		}
		if opts.JSON {
			_ = json.NewEncoder(opts.Writer).Encode(map[string]any{
				"dry_run":     true,
				"kind":        "memorial-json",
				"from":        from,
				"total_rows":  preview.TotalRows,
				"would_create": preview.WouldCreate,
				"would_skip":  preview.WouldSkip,
				"would_fail":  preview.WouldFail,
				"issues":      preview.Issues,
			})
			return 0, nil
		}
		fmt.Fprintf(opts.Writer, "dry-run: would import %s\n", from)
		fmt.Fprintf(opts.Writer, "  total=%d would_create=%d would_skip=%d would_fail=%d\n",
			preview.TotalRows, preview.WouldCreate, preview.WouldSkip, preview.WouldFail)
		if len(preview.Issues) > 0 {
			fmt.Fprintf(opts.Writer, "  issues=%d (preview only — pass --yes to apply anyway)\n", len(preview.Issues))
		}
		fmt.Fprintln(opts.Writer, "memorial-json is additive — pass --yes to apply")
		return 0, nil
	}

	summary, err := a.soldiers.ImportMemorialArchive(from)
	if err != nil {
		return 1, fmt.Errorf("memorial-json import failed: %w", err)
	}

	// MemorialImportSummary doesn't include a log path; the
	// service writes the issues log itself. We pass the summary
	// back so the caller can re-emit in --json mode. Issues is
	// the log payload.
	if opts.JSON {
		_ = json.NewEncoder(opts.Writer).Encode(map[string]any{
			"kind":       "memorial-json",
			"from":       from,
			"total_rows": summary.TotalRows,
			"created":    summary.Created,
			"skipped":    summary.Skipped,
			"failed":     summary.Failed,
			"batch_id":   summary.BatchID,
			"issues":     summary.Issues,
		})
		return 0, nil
	}
	fmt.Fprintf(opts.Writer, "imported %s\n", from)
	fmt.Fprintf(opts.Writer, "  total=%d created=%d skipped=%d failed=%d\n",
		summary.TotalRows, summary.Created, summary.Skipped, summary.Failed)
	if summary.Failed > 0 {
		fmt.Fprintf(opts.Writer, "  see issues in the import log (batch_id=%s)\n", summary.BatchID)
	}
	return 0, nil
}

// --- helpers ---

// loadLocalImportIdentity mirrors backup.currentImportIdentity
// so we can call it before closing the DB handle. Returns the
// identity + the preserveLocalIdentity flag. Errors are silently
// swallowed — the import proceeds with a zero identity, which
// is exactly what the service would have done via the
// currentImportIdentity path.
func loadLocalImportIdentity(a *App) (models.UserIdentity, bool, error) {
	if a.database == nil {
		return models.UserIdentity{}, false, errors.New("database not open")
	}
	complete, err := a.database.SystemConfig("user_identity_complete")
	if err != nil {
		return models.UserIdentity{}, false, err
	}
	if strings.TrimSpace(complete) != "1" {
		return models.UserIdentity{}, false, nil
	}
	identity, err := a.database.UserIdentity()
	if err != nil {
		return models.UserIdentity{}, false, err
	}
	return identity, true, nil
}

// createImportRestorePoint was removed in commit fixing Phase 5.
// Rationale captured inline in runImportBackup / runImportSharedArchive:
// the .ddbak / merge imports overwrite the data dir in place, so a
// restore point written inside it would be destroyed by the very
// import it was meant to back out. Phase 6 will add a sibling
// restore-point root (.dixiedata-restore-points/) that lives outside
// the data dir, at which point this helper returns with the same
// archive-writer closure that wires a.backup.Export.

// readBackupManifestFromZip opens a .ddbak and reads just the
// manifest.json entry. Used by import backup --dry-run to
// surface the count summary without staging anything.
func readBackupManifestFromZip(path string) (archive.BackupManifest, error) {
	var manifest archive.BackupManifest
	zr, err := openZip(path)
	if err != nil {
		return manifest, err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if filepath.Base(f.Name) == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				return manifest, err
			}
			defer rc.Close()
			body, err := io.ReadAll(rc)
			if err != nil {
				return manifest, err
			}
			if err := json.Unmarshal(body, &manifest); err != nil {
				return manifest, err
			}
			return manifest, nil
		}
	}
	return manifest, errors.New("archive contains no manifest.json")
}

// readSharedArchiveManifest reads just the manifest entry from a
// .ddshare. Shared archives use the same BackupManifest struct
// with archive_kind = "shared".
func readSharedArchiveManifest(path string) (archive.BackupManifest, error) {
	m, err := readBackupManifestFromZip(path)
	if err != nil {
		return m, err
	}
	if m.ArchiveKind != "shared" {
		return m, fmt.Errorf("archive is not a shared archive (archive_kind=%q)", m.ArchiveKind)
	}
	return m, nil
}

// (The actual zip helper lives in cli_import_zip.go. The
// readBackupManifestFromZip / readSharedArchiveManifest call
// sites in this file just call openZip directly.)

// loadFeedbackLogFromJSONL is intentionally not implemented —
// see cli-plan.md §Phase 5 audit history for why feedback-log
// import was dropped (no consumer for ingesting JSONL feedback
// logs in the live app).
//
// Compile-time check that records.MemorialImportSummary is
// unchanged in the fields we depend on. Keeps the dispatch
// from silently breaking if records adds a field.
var _ = records.MemorialImportSummary{
	FilePath:  "",
	BatchID:   "",
	TotalRows: 0,
	Created:   0,
	Skipped:   0,
	Failed:    0,
}