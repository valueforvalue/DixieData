package db

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
)

// ErrIdentityAlreadyComplete is returned by ConfigureUserIdentity
// when the system_config row user_identity_complete == "1" and
// the caller did not opt into the explicit-overwrite escape hatch
// via IdentityForceOverwrite. Issue #495: defense-in-depth against
// any code path (test helper, future restore, misrouted handler)
// silently overwriting an already-configured identity — every
// overwrite changes the node_prefix namespace, which renames every
// existing soldier's display_id under the old prefix.
//
// Callers that legitimately need to write the identity a second
// time (backup restore on top of a freshly imported archive, the
// gold-master fixture, the tests/stress helpers) must pass
// IdentityForceOverwrite explicitly so the overwrite is visible in
// the call site.
var ErrIdentityAlreadyComplete = errors.New("identity already configured; pass IdentityForceOverwrite to overwrite")

// SystemConfig returns the system-wide config row (one row per Local Archive).
func (d *DB) SystemConfig(key string) (string, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetSystemConfig updates the system-wide config row.
func (d *DB) SetSystemConfig(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO system_config(key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

// NodePrefix returns the configured per-user node prefix (issue #180). Empty in legacy archives.
func (d *DB) NodePrefix() (string, error) {
	value, err := d.SystemConfig("node_prefix")
	if err != nil {
		return "", err
	}
	return NormalizeNodePrefix(value), nil
}

// BuildUserNodePrefix constructs a node prefix from the user's display name (lowercase, ASCII letters + digits only, truncated).
func BuildUserNodePrefix(firstName, middleName, lastName string, birthYear int) (string, error) {
	firstInitial := firstPrefixInitial(firstName)
	middleInitial := firstPrefixInitial(middleName)
	lastInitial := firstPrefixInitial(lastName)
	if firstInitial == "" || middleInitial == "" || lastInitial == "" {
		return "", fmt.Errorf("first, middle, and last name are required")
	}
	if birthYear < 1000 || birthYear > 9999 {
		return "", fmt.Errorf("birth year must be four digits")
	}
	return firstInitial + middleInitial + lastInitial + fmt.Sprintf("%02d", birthYear%100), nil
}

// UserIdentity returns the per-user identity (node prefix + display name) configured for this Local Archive.
func (d *DB) UserIdentity() (models.UserIdentity, error) {
	var identity models.UserIdentity
	var err error
	identity.FirstName, err = d.SystemConfig("user_first_name")
	if err != nil {
		return models.UserIdentity{}, err
	}
	identity.MiddleName, err = d.SystemConfig("user_middle_name")
	if err != nil {
		return models.UserIdentity{}, err
	}
	identity.LastName, err = d.SystemConfig("user_last_name")
	if err != nil {
		return models.UserIdentity{}, err
	}
	birthYear, err := d.SystemConfig("user_birth_year")
	if err != nil {
		return models.UserIdentity{}, err
	}
	if strings.TrimSpace(birthYear) != "" {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(birthYear))
		if parseErr != nil {
			return models.UserIdentity{}, parseErr
		}
		identity.BirthYear = parsed
	}
	identity.NodePrefix, err = d.NodePrefix()
	if err != nil {
		return models.UserIdentity{}, err
	}
	return identity, nil
}

// IdentitySetupRequired reports whether the first-launch setup wizard still needs to run.
func (d *DB) IdentitySetupRequired() (bool, error) {
	complete, err := d.SystemConfig("user_identity_complete")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(complete) == "1" {
		return false, nil
	}
	var soldierCount int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM soldiers`).Scan(&soldierCount); err != nil {
		return false, err
	}
	return soldierCount == 0, nil
}

// IdentityOption configures ConfigureUserIdentity. Variadic so the
// 100+ existing call sites stay byte-compatible; only the rare
// force-overwrite case opts in.
type IdentityOption func(*identityConfig)

type identityConfig struct {
	forceOverwrite bool
}

// IdentityForceOverwrite opts into overwriting an already-complete
// identity. Use sparingly — every overwrite changes the node_prefix
// namespace, which renames existing soldiers' display_ids under the
// old prefix. Legitimate callers: backup restore on top of a
// freshly-imported archive (internal/archive/backup_service.go),
// the gold-master fixture (cmd/gold-master), tests/stress helpers,
// and configureTestIdentity (internal/appshell/app_test.go).
//
// All other call sites — including the /setup POST handler — must
// omit this option so the existing !a.setupRequired handler-level
// guard gets a defense-in-depth companion in the data layer.
func IdentityForceOverwrite() IdentityOption {
	return func(c *identityConfig) { c.forceOverwrite = true }
}

// ConfigureUserIdentity persists the per-user identity chosen
// during setup. Issue #495: refuses to write if the identity has
// already been configured (user_identity_complete == "1") unless
// the caller opts in via IdentityForceOverwrite. The previous
// implementation accepted any caller and silently overwrote all
// 7 system_config rows, which is what produced the THU00 leak into
// the live .dixiedata on 2026-07-12.
func (d *DB) ConfigureUserIdentity(firstName, middleName, lastName string, birthYear int, opts ...IdentityOption) (models.UserIdentity, error) {
	cfg := identityConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	firstName = strings.TrimSpace(firstName)
	middleName = strings.TrimSpace(middleName)
	lastName = strings.TrimSpace(lastName)
	nodePrefix, err := BuildUserNodePrefix(firstName, middleName, lastName, birthYear)
	if err != nil {
		return models.UserIdentity{}, err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return models.UserIdentity{}, err
	}
	defer tx.Rollback()

	// Issue #495 guard. Runs INSIDE the transaction so the read
	// is consistent with the writes that follow (no TOCTOU window
	// if a concurrent call slips in between the check and the
	// insert). SQLite serializes transactions so this is safe.
	if !cfg.forceOverwrite {
		var complete string
		if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = ?`, "user_identity_complete").Scan(&complete); err != nil && err != sql.ErrNoRows {
			return models.UserIdentity{}, fmt.Errorf("read identity complete flag: %w", err)
		}
		if strings.TrimSpace(complete) == "1" {
			return models.UserIdentity{}, ErrIdentityAlreadyComplete
		}
	}

	fullName := strings.TrimSpace(strings.Join([]string{firstName, middleName, lastName}, " "))
	for key, value := range map[string]string{
		"user_first_name":        firstName,
		"user_middle_name":       middleName,
		"user_last_name":         lastName,
		"user_birth_year":        fmt.Sprintf("%04d", birthYear),
		"user_display_name":      fullName,
		"user_identity_complete": "1",
		"node_prefix":            nodePrefix,
	} {
		if _, err := tx.Exec(`
			INSERT INTO system_config(key, value)
			VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET
				value = excluded.value,
				updated_at = CURRENT_TIMESTAMP
		`, key, value); err != nil {
			return models.UserIdentity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.UserIdentity{}, err
	}
	return models.UserIdentity{
		FirstName:  firstName,
		MiddleName: middleName,
		LastName:   lastName,
		BirthYear:  birthYear,
		NodePrefix: nodePrefix,
	}, nil
}

// BackfillEntryAuditIdentity sets the audit identity (node_prefix + display_id) on every Soldiers row that lacks it. Used after ConfigureUserIdentity on legacy archives.
func (d *DB) BackfillEntryAuditIdentity() error {
	identity, err := d.UserIdentity()
	if err != nil {
		return err
	}
	actor := strings.TrimSpace(identity.BrandingName())
	if actor == "" {
		return nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE soldiers SET added_by = ? WHERE added_by IS NULL OR TRIM(added_by) = ''`, actor); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET last_edited_by = ? WHERE last_edited_by IS NULL OR TRIM(last_edited_by) = ''`, actor); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE soldiers SET last_edited_at = COALESCE(NULLIF(updated_at, ''), NULLIF(created_at, ''), CURRENT_TIMESTAMP) WHERE last_edited_at IS NULL OR TRIM(last_edited_at) = ''`); err != nil {
		return err
	}
	return tx.Commit()
}

// EntryAuditIdentityBackfillNeeded reports whether the entry-audit-identity backfill still has rows to process.
func (d *DB) EntryAuditIdentityBackfillNeeded() (bool, error) {
	var needed int
	if err := d.conn.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM soldiers
			WHERE added_by IS NULL OR TRIM(added_by) = ''
				OR last_edited_by IS NULL OR TRIM(last_edited_by) = ''
				OR last_edited_at IS NULL OR TRIM(last_edited_at) = ''
			LIMIT 1
		)`).Scan(&needed); err != nil {
		return false, err
	}
	return needed == 1, nil
}

func firstPrefixInitial(value string) string {
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return string(r)
		}
	}
	return ""
}

// NewSyncID returns a fresh sync ID (UUIDv4) for a newly-created row. Sync IDs are stable across import/export so a row that round-trips through a Shared Archive re-merges correctly.
func NewSyncID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x70
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
