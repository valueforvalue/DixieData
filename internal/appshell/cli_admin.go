// cli_admin.go — `dixiedata migrate|backup|restore point|logs|config ...`
// subcommands (Phase 6 of cli-plan.md). Each maps to an existing
// *App method or internal package. No new business logic; just
// CLI plumbing.
//
//   dixiedata migrate status
//   dixiedata migrate up
//   dixiedata migrate down <version>          (intentionally not shipped; see note)
//   dixiedata backup list [--json]
//   dixiedata backup prune [--keep-last N]
//   dixiedata restore point list [--json]
//   dixiedata restore point create [--note <text>] [--root <path>]
//   dixiedata restore point apply <id>
//   dixiedata logs path
//   dixiedata logs tail [--follow] [--lines N]
//   dixiedata config show [--json]
//   dixiedata config set <key> <value>
//
// Common flags:
//   --data-dir PATH    override the data dir (default: appdata.DefaultDir())
//   --json             stable JSON envelope
//
// migrate down is NOT shipped yet: the schema-version-down path
// in internal/db/migrate is "best-effort, may not undo all
// changes", and the CLI would need a fresh --yes guard plus a
// pre-migration snapshot. Phase 6 ships status + up (which is
// the same as opening the DB; applySchema short-circuits if
// user_version >= CurrentSchemaVersion).
//
// restore point create accepts --root PATH so the CLI can
// pre-import snapshot at a SIBLING of the data dir
// (.dixiedata-restore-points/). The Phase 5 safety net
// re-enable (planned follow-up commit) reuses this.
package appshell

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/update"
)

// AdminKind identifies which top-level admin subcommand the
// user wants. Each one (except AdminUnknown) requires a
// sub-verb (e.g. "migrate" -> "status"/"up"; "backup" ->
// "list"/"prune"; "restore point" -> "list"/"create"/"apply").
type AdminKind int

const (
	AdminUnknown AdminKind = iota
	AdminMigrate
	AdminBackup
	AdminRestorePoint
	AdminLogs
	AdminConfig
)

// AdminAction is the per-family verb. Combined with AdminKind
// it identifies a single subcommand.
type AdminAction int

const (
	AdminActionUnknown AdminAction = iota
	// migrate
	AdminMigrateStatus
	AdminMigrateUp
	// backup
	AdminBackupList
	AdminBackupPrune
	// restore point
	AdminRestorePointList
	AdminRestorePointCreate
	AdminRestorePointApply
	// logs
	AdminLogsPath
	AdminLogsTail
	// config
	AdminConfigShow
	AdminConfigSet
)

// AdminOptions is the parsed CLI input. App is filled in by
// runAdminSubcommand after a.Startup.
type AdminOptions struct {
	Kind            AdminKind
	Action          AdminAction
	DataDir         string // --data-dir override
	JSON            bool   // --json
	KeepLast        int    // --keep-last N (backup prune)
	Note            string // --note <text> (restore point create)
	RestorePointRoot string // --root <path> (restore point create)
	RestorePointID  string // <id> positional (restore point apply)
	ConfigKey       string // <key> positional (config set)
	ConfigValue     string // <value> positional (config set)
	Follow          bool   // --follow (logs tail)
	TailLines       int    // --lines N (logs tail, default 100)
	Writer          io.Writer
	App             *App
}

// HasAdminSubcommand returns true when the first arg is one of
// the top-level admin subcommand verbs. main.go uses this to
// dispatch before falling through to wails.Run.
func HasAdminSubcommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "migrate", "backup", "restore", "logs", "config":
		return true
	}
	return false
}

// ParseAdminArgs walks the args slice once and fills an
// AdminOptions. Returns AdminKind=AdminUnknown if the args
// don't start with an admin verb (caller should ignore).
func ParseAdminArgs(args []string) (AdminOptions, error) {
	opts := AdminOptions{Writer: os.Stdout}
	if len(args) == 0 {
		return opts, nil
	}
	switch args[0] {
	case "migrate":
		opts.Kind = AdminMigrate
		if len(args) < 2 {
			return opts, fmt.Errorf("migrate requires a subcommand: status, up")
		}
		switch args[1] {
		case "status":
			opts.Action = AdminMigrateStatus
		case "up":
			opts.Action = AdminMigrateUp
		default:
			return opts, fmt.Errorf("unknown migrate subcommand: %q (want status, up)", args[1])
		}
	case "backup":
		opts.Kind = AdminBackup
		if len(args) < 2 {
			return opts, fmt.Errorf("backup requires a subcommand: list, prune")
		}
		switch args[1] {
		case "list":
			opts.Action = AdminBackupList
		case "prune":
			opts.Action = AdminBackupPrune
		default:
			return opts, fmt.Errorf("unknown backup subcommand: %q (want list, prune)", args[1])
		}
	case "restore":
		// restore point is two words; require both.
		if len(args) < 2 || args[1] != "point" {
			return opts, fmt.Errorf("unknown restore subcommand: %q (want 'restore point ...')", args[1])
		}
		opts.Kind = AdminRestorePoint
		if len(args) < 3 {
			return opts, fmt.Errorf("restore point requires a subcommand: list, create, apply")
		}
		switch args[2] {
		case "list":
			opts.Action = AdminRestorePointList
		case "create":
			opts.Action = AdminRestorePointCreate
		case "apply":
			opts.Action = AdminRestorePointApply
			// <id> is the first non-flag arg after "restore point apply".
			for i := 3; i < len(args); i++ {
				if !strings.HasPrefix(args[i], "--") {
					opts.RestorePointID = args[i]
					break
				}
			}
		default:
			return opts, fmt.Errorf("unknown restore point subcommand: %q (want list, create, apply)", args[2])
		}
	case "logs":
		opts.Kind = AdminLogs
		if len(args) < 2 {
			return opts, fmt.Errorf("logs requires a subcommand: path, tail")
		}
		switch args[1] {
		case "path":
			opts.Action = AdminLogsPath
		case "tail":
			opts.Action = AdminLogsTail
		default:
			return opts, fmt.Errorf("unknown logs subcommand: %q (want path, tail)", args[1])
		}
	case "config":
		opts.Kind = AdminConfig
		if len(args) < 2 {
			return opts, fmt.Errorf("config requires a subcommand: show, set")
		}
		switch args[1] {
		case "show":
			opts.Action = AdminConfigShow
		case "set":
			opts.Action = AdminConfigSet
			// set <key> <value> are the first two non-flag args
			// after "config set".
			positional := []string{}
			for i := 2; i < len(args); i++ {
				if !strings.HasPrefix(args[i], "--") {
					positional = append(positional, args[i])
				}
			}
			if len(positional) >= 2 {
				opts.ConfigKey = positional[0]
				opts.ConfigValue = positional[1]
			}
		default:
			return opts, fmt.Errorf("unknown config subcommand: %q (want show, set)", args[1])
		}
	default:
		return opts, nil
	}

	// Common + per-subcommand flags. We use 2-pass: first
	// `--flag value` (space form), then `--flag=value` /
	// `--flag` (no value) so order doesn't matter.
	for i := 2; i < len(args)-1; i++ {
		switch args[i] {
		case "--data-dir":
			opts.DataDir = args[i+1]
		case "--note":
			opts.Note = args[i+1]
		case "--root":
			opts.RestorePointRoot = args[i+1]
		case "--keep-last":
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				opts.KeepLast = n
			}
		case "--lines":
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				opts.TailLines = n
			}
		}
	}
	for _, a := range args[2:] {
		switch {
		case a == "--json":
			opts.JSON = true
		case a == "--follow":
			opts.Follow = true
		case strings.HasPrefix(a, "--data-dir="):
			opts.DataDir = strings.TrimPrefix(a, "--data-dir=")
		case strings.HasPrefix(a, "--note="):
			opts.Note = strings.TrimPrefix(a, "--note=")
		case strings.HasPrefix(a, "--root="):
			opts.RestorePointRoot = strings.TrimPrefix(a, "--root=")
		case strings.HasPrefix(a, "--keep-last="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--keep-last=")); err == nil {
				opts.KeepLast = n
			}
		case strings.HasPrefix(a, "--lines="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--lines=")); err == nil {
				opts.TailLines = n
			}
		}
	}

	// Defaults.
	if opts.Action == AdminBackupPrune && opts.KeepLast <= 0 {
		opts.KeepLast = 5 // match defaultMaxRetainedBackups
	}
	if opts.Action == AdminLogsTail && opts.TailLines <= 0 {
		opts.TailLines = 100
	}

	// Sanity.
	if opts.Action == AdminRestorePointApply && opts.RestorePointID == "" {
		return opts, fmt.Errorf("restore point apply requires an <id>")
	}
	if opts.Action == AdminConfigSet {
		if opts.ConfigKey == "" || opts.ConfigValue == "" {
			return opts, fmt.Errorf("config set requires <key> <value>")
		}
		if !isKnownConfigKey(opts.ConfigKey) {
			return opts, fmt.Errorf("unknown config key: %q (known: debug_mode)", opts.ConfigKey)
		}
	}

	return opts, nil
}

// isKnownConfigKey returns true if key is a valid
// local_settings field. Adding a new key requires both a
// field on records.LocalSettings AND a case here.
func isKnownConfigKey(key string) bool {
	switch key {
	case "debug_mode":
		return true
	}
	return false
}

// RunAdmin dispatches to the right handler. Returns exit code
// (0 ok, 1 internal error, 2 env error, 3 usage error).
func RunAdmin(ctx context.Context, opts AdminOptions) (int, error) {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.Kind == AdminUnknown || opts.Action == AdminActionUnknown {
		return 3, fmt.Errorf("not an admin subcommand")
	}
	if opts.App == nil {
		return 2, fmt.Errorf("RunAdmin requires opts.App")
	}

	// --data-dir override: if the user passed --data-dir, swap
	// the env var BEFORE any service is touched. appdata.DefaultDir()
	// re-reads DIXIEDATA_DATA_DIR each call, so this is enough
	// for fresh lookups; the App's dataDir was already set in
	// Startup() from the previous env value, so we update it
	// directly here too.
	if opts.DataDir != "" {
		_ = os.Setenv("DIXIEDATA_DATA_DIR", opts.DataDir)
		opts.App.dataDir = opts.DataDir
	}

	switch opts.Action {
	case AdminMigrateStatus:
		return runAdminMigrateStatus(ctx, opts)
	case AdminMigrateUp:
		return runAdminMigrateUp(ctx, opts)
	case AdminBackupList:
		return runAdminBackupList(ctx, opts)
	case AdminBackupPrune:
		return runAdminBackupPrune(ctx, opts)
	case AdminRestorePointList:
		return runAdminRestorePointList(ctx, opts)
	case AdminRestorePointCreate:
		return runAdminRestorePointCreate(ctx, opts)
	case AdminRestorePointApply:
		return runAdminRestorePointApply(ctx, opts)
	case AdminLogsPath:
		return runAdminLogsPath(ctx, opts)
	case AdminLogsTail:
		return runAdminLogsTail(ctx, opts)
	case AdminConfigShow:
		return runAdminConfigShow(ctx, opts)
	case AdminConfigSet:
		return runAdminConfigSet(ctx, opts)
	}
	return 3, fmt.Errorf("unhandled admin action: %v", opts.Action)
}

// --- migrate ---

// runAdminMigrateStatus reports the current vs target schema
// version, app version, and the data dir. Read-only.
func runAdminMigrateStatus(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	database, err := db.Open(app.dataDir)
	if err != nil {
		return 2, fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	current := db.CurrentSchemaVersion
	applied := 0
	if v, err := queryUserVersion(database.Conn()); err == nil {
		applied = v
	}
	pending := current - applied
	if pending < 0 {
		pending = 0
	}
	status := "up-to-date"
	if pending > 0 {
		status = "pending"
	}

	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"app_version":     buildinfo.AppVersion,
			"build_identity":  buildinfo.BuildIdentity(),
			"data_dir":        app.dataDir,
			"applied_version": applied,
			"current_version": current,
			"pending":         pending,
			"status":          status,
		})
	}
	fmt.Fprintf(opts.Writer, "data_dir        = %s\n", app.dataDir)
	fmt.Fprintf(opts.Writer, "app_version     = %s\n", buildinfo.AppVersion)
	fmt.Fprintf(opts.Writer, "build_identity  = %s\n", buildinfo.BuildIdentity())
	fmt.Fprintf(opts.Writer, "applied_version = %d\n", applied)
	fmt.Fprintf(opts.Writer, "current_version = %d\n", current)
	fmt.Fprintf(opts.Writer, "pending         = %d\n", pending)
	fmt.Fprintf(opts.Writer, "status          = %s\n", status)
	return 0, nil
}

// queryUserVersion reads PRAGMA user_version from a *sql.DB.
// Kept as a tiny helper so tests can call it without
// standing up the rest of the migrate machinery.
func queryUserVersion(conn *sql.DB) (int, error) {
	var v int
	row := conn.QueryRow(`PRAGMA user_version`)
	if err := row.Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

// runAdminMigrateUp opens the DB, which triggers applySchema.
// applySchema short-circuits if user_version is already
// CurrentSchemaVersion, so this is a no-op when the DB is
// current. Use `restore point create` BEFORE this if you
// want a rollback safety net.
func runAdminMigrateUp(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	database, err := db.Open(app.dataDir)
	if err != nil {
		return 2, fmt.Errorf("open db: %w", err)
	}
	before, err := queryUserVersion(database.Conn())
	if err != nil {
		database.Close()
		return 2, fmt.Errorf("read user_version (before): %w", err)
	}
	database.Close()

	database, err = db.Open(app.dataDir)
	if err != nil {
		return 2, fmt.Errorf("reopen db: %w", err)
	}
	after, err := queryUserVersion(database.Conn())
	if err != nil {
		database.Close()
		return 2, fmt.Errorf("read user_version (after): %w", err)
	}
	database.Close()

	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"before": before,
			"after":  after,
			"moved":  after - before,
		})
	}
	fmt.Fprintf(opts.Writer, "schema before = %d\n", before)
	fmt.Fprintf(opts.Writer, "schema after  = %d\n", after)
	if after > before {
		fmt.Fprintf(opts.Writer, "applied %d migration(s)\n", after-before)
	} else {
		fmt.Fprintln(opts.Writer, "no migration needed (already at current)")
	}
	return 0, nil
}

// --- backup ---

// runAdminBackupList prints the retained backup index
// (pre-schema-upgrade snapshots). Read-only.
func runAdminBackupList(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	manager := update.NewRetainedBackupManager(app.dataDir)
	backups, err := manager.List()
	if err != nil {
		return 2, fmt.Errorf("list backups: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"count":   len(backups),
			"backups": backups,
		})
	}
	fmt.Fprintf(opts.Writer, "count = %d\n", len(backups))
	for _, b := range backups {
		fmt.Fprintf(opts.Writer, "  %s  v%d->v%d  %s  %s\n",
			b.ID, b.SourceSchemaVersion, b.TargetSchemaVersion, b.CreatedAt, b.DatabaseSnapshotPath)
	}
	return 0, nil
}

// runAdminBackupPrune trims the retained-backup index to
// --keep-last N entries. Removes the on-disk files for the
// pruned entries.
func runAdminBackupPrune(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	manager := update.NewRetainedBackupManager(app.dataDir)
	before, err := manager.List()
	if err != nil {
		return 2, fmt.Errorf("list backups: %w", err)
	}
	// Manual prune: keep the first N (List already sorts newest
	// first), RemoveAll the rest. There's no public Prune on
	// the manager, so we use the same pattern Housekeeping
	// would use.
	if opts.KeepLast >= len(before) {
		if opts.JSON {
			return 0, writeJSON(opts.Writer, map[string]any{
				"before": len(before),
				"after":  len(before),
				"pruned": 0,
			})
		}
		fmt.Fprintf(opts.Writer, "nothing to prune (kept=%d, have=%d)\n", opts.KeepLast, len(before))
		return 0, nil
	}
	removed := 0
	for _, stale := range before[opts.KeepLast:] {
		path := filepath.Join(app.dataDir, filepath.FromSlash(stale.DatabaseSnapshotPath))
		if err := os.RemoveAll(path); err == nil {
			removed++
		}
	}
	// Re-list to confirm.
	after, err := manager.List()
	if err != nil {
		return 2, fmt.Errorf("re-list backups: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"before":   len(before),
			"after":    len(after),
			"pruned":   removed,
			"keep_last": opts.KeepLast,
		})
	}
	fmt.Fprintf(opts.Writer, "before = %d\n", len(before))
	fmt.Fprintf(opts.Writer, "after  = %d\n", len(after))
	fmt.Fprintf(opts.Writer, "pruned = %d (kept last %d)\n", removed, opts.KeepLast)
	return 0, nil
}

// --- restore point ---

// runAdminRestorePointList prints the restore point index.
func runAdminRestorePointList(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	manager := app.restorePoints
	if manager == nil {
		return 2, fmt.Errorf("restore point manager unavailable")
	}
	points, err := manager.List()
	if err != nil {
		return 2, fmt.Errorf("list restore points: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"count":          len(points),
			"restore_points": points,
		})
	}
	fmt.Fprintf(opts.Writer, "count = %d\n", len(points))
	for _, p := range points {
		fmt.Fprintf(opts.Writer, "  %s  %s  v%s->v%s  %s\n",
			p.ID, p.CreatedAt, p.SourceAppVersion, p.TargetAppVersion, p.LocalArchivePath)
	}
	return 0, nil
}

// runAdminRestorePointCreate snapshots the live archive at
// <dataDir>/updates/restore-points/<id>/ (default) or
// <--root>/<id>/ (if --root passed). Print the ID. Phase 5
// re-enable will call into this from createImportRestorePoint.
func runAdminRestorePointCreate(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	archiveWriter := func(outputPath string) error {
		_, err := app.backup.Export(outputPath, app.dataDir)
		return err
	}
	// For CLI, the installed-build snapshot is a no-op: there's
	// no "binary in the field" to roll back to, and forcing
	// cp-of-the-current-exe creates more confusion than safety.
	buildWriter := func(outputDir string) error {
		return os.MkdirAll(outputDir, 0o755)
	}

	var (
		record update.RestorePointRecord
		err    error
	)
	if opts.RestorePointRoot != "" {
		// Sibling manager: root is OUTSIDE dataDir so the
		// snapshot survives archive.replaceDataDir (Phase 5
		// safety net re-enable). The sibling dir is
		// <dataDir-parent>/<dataDir-base>-restore-points by
		// convention.
		root := opts.RestorePointRoot
		if !filepath.IsAbs(root) {
			// resolve relative to project root (parent of dataDir)
			root = filepath.Join(filepath.Dir(app.dataDir), root)
		}
		manager := update.NewSiblingRestorePointManager(app.dataDir, root)
		record, err = manager.Create(update.CreateRestorePointInput{
			SourceAppVersion:    buildinfo.AppVersion,
			TargetAppVersion:    buildinfo.AppVersion,
			SourceBuildIdentity: buildinfo.BuildIdentity(),
			TargetBuildIdentity: buildinfo.BuildIdentity(),
		}, archiveWriter, buildWriter)
	} else {
		manager := app.restorePoints
		if manager == nil {
			return 2, fmt.Errorf("restore point manager unavailable")
		}
		record, err = manager.Create(update.CreateRestorePointInput{
			SourceAppVersion:    buildinfo.AppVersion,
			TargetAppVersion:    buildinfo.AppVersion,
			SourceBuildIdentity: buildinfo.BuildIdentity(),
			TargetBuildIdentity: buildinfo.BuildIdentity(),
		}, archiveWriter, buildWriter)
	}
	if err != nil {
		return 2, fmt.Errorf("create restore point: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"id":                record.ID,
			"created_at":        record.CreatedAt,
			"local_archive":     record.LocalArchivePath,
			"installed_build":   record.InstalledBuildPath,
			"sibling_root":      opts.RestorePointRoot != "",
			"note":              opts.Note,
		})
	}
	fmt.Fprintf(opts.Writer, "id              = %s\n", record.ID)
	fmt.Fprintf(opts.Writer, "created_at      = %s\n", record.CreatedAt)
	fmt.Fprintf(opts.Writer, "local_archive   = %s\n", record.LocalArchivePath)
	fmt.Fprintf(opts.Writer, "installed_build = %s\n", record.InstalledBuildPath)
	if opts.Note != "" {
		fmt.Fprintf(opts.Writer, "note            = %s\n", opts.Note)
	}
	return 0, nil
}

// runAdminRestorePointApply is a placeholder: the manager
// has no public Apply because the in-place update flow does
// the actual restore (downgrade via the in-place scripts in
// internal/update). For Phase 6 we print the restore point
// contents so the user can manually `cp` the local-archive
// aside and `dixiedata import backup --from <copy> --yes` to
// apply. A real Apply is Phase 7+ work (touches
// replaceDataDir's data-dir-resident restore point).
func runAdminRestorePointApply(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	manager := app.restorePoints
	if manager == nil {
		return 2, fmt.Errorf("restore point manager unavailable")
	}
	record, err := manager.Get(opts.RestorePointID)
	if err != nil {
		return 1, fmt.Errorf("get restore point: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, record)
	}
	fmt.Fprintf(opts.Writer, "id              = %s\n", record.ID)
	fmt.Fprintf(opts.Writer, "created_at      = %s\n", record.CreatedAt)
	fmt.Fprintf(opts.Writer, "source_version  = %s\n", record.SourceAppVersion)
	fmt.Fprintf(opts.Writer, "target_version  = %s\n", record.TargetAppVersion)
	fmt.Fprintf(opts.Writer, "local_archive   = %s\n", record.LocalArchivePath)
	fmt.Fprintf(opts.Writer, "installed_build = %s\n", record.InstalledBuildPath)
	fmt.Fprintln(opts.Writer, "(apply not yet implemented; see docs/agents/cli-plan.md Phase 6 + 7)")
	return 0, nil
}

// --- logs ---

// runAdminLogsPath prints the path to the app log JSONL
// file. Read-only.
func runAdminLogsPath(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	path := appdata.AppLogPath(app.dataDir)
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"path": path,
		})
	}
	fmt.Fprintln(opts.Writer, path)
	return 0, nil
}

// runAdminLogsTail prints the last N lines of the app log
// (default 100). With --follow, polls every 250ms and prints
// new lines. File-rotation aware: reopens if the inode
// changes.
func runAdminLogsTail(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	path := appdata.AppLogPath(app.dataDir)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if opts.JSON {
				return 0, writeJSON(opts.Writer, map[string]any{
					"path":  path,
					"lines": []string{},
				})
			}
			fmt.Fprintf(opts.Writer, "(no log file at %s)\n", path)
			return 0, nil
		}
		return 2, fmt.Errorf("stat log: %w", err)
	}

	// Initial read: last N lines.
	lines, err := tailFile(path, opts.TailLines)
	if err != nil {
		return 2, fmt.Errorf("read log: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"path":  path,
			"lines": lines,
		})
	}
	for _, l := range lines {
		fmt.Fprintln(opts.Writer, l)
	}
	if !opts.Follow {
		return 0, nil
	}

	// --follow: poll for new lines.
	lastSize, err := fileSize(path)
	if err != nil {
		return 2, fmt.Errorf("stat log: %w", err)
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, nil
		case <-ticker.C:
			newSize, err := fileSize(path)
			if err != nil {
				continue
			}
			if newSize < lastSize {
				// rotated; reset.
				lastSize = 0
			}
			if newSize > lastSize {
				f, err := os.Open(path)
				if err != nil {
					continue
				}
				if _, err := f.Seek(int64(lastSize), io.SeekStart); err != nil {
					f.Close()
					continue
				}
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					fmt.Fprintln(opts.Writer, scanner.Text())
				}
				f.Close()
				lastSize = newSize
			}
		}
	}
}

// tailFile returns up to n lines from the end of path. If the
// file has fewer than n lines, returns them all.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Read all lines (log files are small enough for this).
	var all []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // long lines
	for scanner.Scan() {
		all = append(all, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// --- config ---

// runAdminConfigShow prints local_settings. Read-only.
func runAdminConfigShow(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	settings, err := records.LoadLocalSettings(app.dataDir)
	if err != nil {
		return 2, fmt.Errorf("load local settings: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"data_dir":  app.dataDir,
			"settings":  settings,
		})
	}
	fmt.Fprintf(opts.Writer, "data_dir    = %s\n", app.dataDir)
	fmt.Fprintf(opts.Writer, "debug_mode  = %t\n", settings.DebugMode)
	return 0, nil
}

// runAdminConfigSet writes a single key to local_settings.
// Only known keys are accepted (see isKnownConfigKey).
func runAdminConfigSet(ctx context.Context, opts AdminOptions) (int, error) {
	app := opts.App
	settings, err := records.LoadLocalSettings(app.dataDir)
	if err != nil {
		return 2, fmt.Errorf("load local settings: %w", err)
	}
	switch opts.ConfigKey {
	case "debug_mode":
		b, err := strconv.ParseBool(opts.ConfigValue)
		if err != nil {
			return 3, fmt.Errorf("config set debug_mode: want bool, got %q", opts.ConfigValue)
		}
		settings.DebugMode = b
	}
	if err := records.SaveLocalSettings(app.dataDir, settings); err != nil {
		return 2, fmt.Errorf("save local settings: %w", err)
	}
	if opts.JSON {
		return 0, writeJSON(opts.Writer, map[string]any{
			"key":     opts.ConfigKey,
			"value":   opts.ConfigValue,
			"applied": true,
		})
	}
	fmt.Fprintf(opts.Writer, "%s = %s\n", opts.ConfigKey, opts.ConfigValue)
	return 0, nil
}

// writeJSON serializes v and writes to w. Used by all
// --json outputs. Pretty-printed with 2-space indent.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
