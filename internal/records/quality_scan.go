package records

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/models"
)

type DataQualityMode string

const (
	DataQualityModeHighConfidence DataQualityMode = "high-confidence"
	DataQualityModeAdvanced       DataQualityMode = "advanced"

	qualityReviewReasonMarker = "Heuristic scan flagged data-quality issues."
)

type DataQualityIssue struct {
	SoldierID         int64
	DisplayID         string
	Name              string
	EntryType         string
	Group             string
	Code              string
	Severity          string
	Summary           string
	Detail            string
	// Issue #377 / #423: row provenance fields (slice 3 of #423
	// surfaces them in the data-quality scan results so the user
	// can see at a glance whether a flagged row's corruption came
	// from an external import vs. a local edit). ImportPath is
	// the code path that wrote the row (e.g. "create_soldier",
	// "memorial_json_import", "restore_backup_archive"); RestoredAt
	// is the RFC3339 timestamp the row was carried over the most
	// recent SQLite-snapshot restore (empty = never restored).
	ImportPath         string
	RestoredAt         string
}

// DataQualityScanResult is a records-layer type used by the matching service.
type DataQualityScanResult struct {
	Mode           DataQualityMode
	ScannedRecords int
	Issues         []DataQualityIssue
}

// DataQualityApplyResult is a records-layer type used by the matching service.
type DataQualityApplyResult struct {
	Selected       int
	Flagged        int
	AlreadyInQueue int
	NotFound       int
}

type qualityScanCandidate struct {
	ID              int64
	DisplayID       string
	EntryType       string
	SpouseSoldierID int64
	FirstName       string
	MiddleName      string
	LastName        string
	BirthDate       string
	DeathDate       string
	BirthInfo       string
	BuriedIn        string
	// Issue #377 / #423: row provenance fields carried from
	// the soldiers table into the scan candidate so every
	// DataQualityIssue can be stamped with the row's origin
	// + restore history.
	ImportPath      string
	RestoredAt      string
}

func normalizeDataQualityMode(raw string) DataQualityMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(DataQualityModeAdvanced):
		return DataQualityModeAdvanced
	default:
		return DataQualityModeHighConfidence
	}
}

// RunDataQualityScan runs the data-quality scan (per DataQualityMode) over every Soldier; returns the per-kind issue-group rollup.
func (s *SoldierService) RunDataQualityScan(modeRaw string) (DataQualityScanResult, error) {
	mode := normalizeDataQualityMode(modeRaw)
	candidates, err := s.loadQualityScanCandidates()
	if err != nil {
		return DataQualityScanResult{}, err
	}

	issues := make([]DataQualityIssue, 0)
	spouseTypes, err := s.loadEntryTypesByID()
	if err != nil {
		return DataQualityScanResult{}, err
	}

	for _, candidate := range candidates {
		issues = append(issues, evaluateQualityIssues(candidate, spouseTypes, mode)...)
	}

	// v60 (issue #320): Event Records with zero links to any
	// Person Record are review-queue candidates. The Event exists
	// but is orphaned (the researcher created it but hasn't
	// attached it to anyone yet, or the attached Person Records
	// were all deleted). This check fires for every Event Record
	// regardless of scan mode (the zero-link condition is a
	// structural integrity issue, not a content-quality issue).
	eventLinkIssues, err := s.loadEventZeroLinkIssues()
	if err != nil {
		return DataQualityScanResult{}, err
	}
	issues = append(issues, eventLinkIssues...)

	if mode == DataQualityModeAdvanced {
		advancedIssues, err := s.loadAdvancedSourceRecordIssues()
		if err != nil {
			return DataQualityScanResult{}, err
		}
		issues = append(issues, advancedIssues...)
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Group != issues[j].Group {
			return issues[i].Group < issues[j].Group
		}
		if issues[i].DisplayID != issues[j].DisplayID {
			return issues[i].DisplayID < issues[j].DisplayID
		}
		return issues[i].Code < issues[j].Code
	})

	return DataQualityScanResult{
		Mode:           mode,
		ScannedRecords: len(candidates),
		Issues:         issues,
	}, nil
}

// ApplyDataQualityFindingsToReviewQueue turns one DataQualityIssueGroup into a Review Queue entry for the user to resolve manually.
func (s *SoldierService) ApplyDataQualityFindingsToReviewQueue(ids []int64) (DataQualityApplyResult, error) {
	result := DataQualityApplyResult{}
	uniqueIDs := dedupePositiveIDs(ids)
	result.Selected = len(uniqueIDs)
	for _, id := range uniqueIDs {
		var (
			needsReview bool
			reason      string
		)
		err := s.db.Conn().QueryRow(`SELECT needs_review, COALESCE(review_reason, '') FROM soldiers WHERE id = ?`, id).Scan(&needsReview, &reason)
		if err != nil {
			if err == sql.ErrNoRows {
				result.NotFound++
				continue
			}
			return DataQualityApplyResult{}, err
		}

		nextReason := mergeQualityReviewReason(reason)
		if needsReview {
			if strings.TrimSpace(nextReason) != strings.TrimSpace(reason) {
				if _, err := s.db.Conn().Exec(`UPDATE soldiers SET review_reason = ? WHERE id = ?`, nextReason, id); err != nil {
					return DataQualityApplyResult{}, err
				}
				if err := s.touchAuditFields(id, "review_status"); err != nil {
					return DataQualityApplyResult{}, err
				}
			}
			result.AlreadyInQueue++
			continue
		}

		if err := s.SetReviewStatus(id, true, nextReason); err != nil {
			return DataQualityApplyResult{}, err
		}
		result.Flagged++
	}
	return result, nil
}

func mergeQualityReviewReason(existing string) string {
	trimmed := strings.TrimSpace(existing)
	if trimmed == "" {
		return qualityReviewReasonMarker
	}
	if strings.Contains(strings.ToLower(trimmed), strings.ToLower(qualityReviewReasonMarker)) {
		return trimmed
	}
	return trimmed + " | " + qualityReviewReasonMarker
}

func dedupePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id < 1 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func (s *SoldierService) loadQualityScanCandidates() ([]qualityScanCandidate, error) {
	rows, err := s.db.Conn().Query(`
		SELECT id, display_id, entry_type, COALESCE(spouse_soldier_id, 0),
		       COALESCE(first_name, ''), COALESCE(middle_name, ''), COALESCE(last_name, ''),
		       COALESCE(birth_date, ''), COALESCE(death_date, ''),
		       COALESCE(birth_info, ''), COALESCE(buried_in, ''),
		       COALESCE(created_by_import_path, ''), COALESCE(restored_at, '')
		FROM soldiers`)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "loadQualityScanCandidates.rows")

	candidates := make([]qualityScanCandidate, 0)
	for rows.Next() {
		var candidate qualityScanCandidate
		if err := rows.Scan(
			&candidate.ID, &candidate.DisplayID, &candidate.EntryType, &candidate.SpouseSoldierID,
			&candidate.FirstName, &candidate.MiddleName, &candidate.LastName,
			&candidate.BirthDate, &candidate.DeathDate,
			&candidate.BirthInfo, &candidate.BuriedIn,
			&candidate.ImportPath, &candidate.RestoredAt,
		); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *SoldierService) loadEntryTypesByID() (map[int64]string, error) {
	rows, err := s.db.Conn().Query(`SELECT id, COALESCE(entry_type, 'soldier') FROM soldiers`)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "loadEntryTypesByID.rows")
	results := map[int64]string{}
	for rows.Next() {
		var id int64
		var entryType string
		if err := rows.Scan(&id, &entryType); err != nil {
			return nil, err
		}
		results[id] = normalizeEntryType(entryType)
	}
	return results, rows.Err()
}

func (s *SoldierService) loadAdvancedSourceRecordIssues() ([]DataQualityIssue, error) {
	rows, err := s.db.Conn().Query(`
		SELECT s.id, COALESCE(s.display_id, ''), COALESCE(s.first_name, ''), COALESCE(s.middle_name, ''), COALESCE(s.last_name, ''), COUNT(r.id),
		       COALESCE(s.created_by_import_path, ''), COALESCE(s.restored_at, '')
		FROM soldiers s
		JOIN records r ON r.person_record_id = s.id
		WHERE TRIM(COALESCE(r.record_type, '')) = ''
		  AND TRIM(COALESCE(r.app_id, '')) = ''
		  AND TRIM(COALESCE(r.details, '')) = ''
		GROUP BY s.id, s.display_id, s.first_name, s.middle_name, s.last_name, s.created_by_import_path, s.restored_at`)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "loadAdvancedSourceRecordIssues.rows")

	issues := make([]DataQualityIssue, 0)
	for rows.Next() {
		var (
			id               int64
			displayID        string
			firstName        string
			middleName       string
			lastName         string
			incompleteRecord int
			importPath       string
			restoredAt       string
		)
		if err := rows.Scan(&id, &displayID, &firstName, &middleName, &lastName, &incompleteRecord, &importPath, &restoredAt); err != nil {
			return nil, err
		}
		issues = append(issues, DataQualityIssue{
			SoldierID:  id,
			DisplayID:  strings.TrimSpace(displayID),
			Name:       buildIssueName(firstName, middleName, lastName),
			Group:      "Source Records",
			Code:       "source-record-empty",
			Severity:   "medium",
			Summary:    "One or more source records are effectively blank.",
			Detail:     fmt.Sprintf("%d source record row(s) have empty type, app ID, and details.", incompleteRecord),
			ImportPath: strings.TrimSpace(importPath),
			RestoredAt: strings.TrimSpace(restoredAt),
		})
	}
	return issues, rows.Err()
}

func candidateIssue(candidate qualityScanCandidate, name, entryType, group, code, severity, summary, detail string) DataQualityIssue {
	// Issue #377 / #423: stamp the row's provenance fields so the
	// data-quality scan results can show "via memorial_json_import,
	// restored at 2026-07-08" next to each flagged row's name.
	// ImportPath and RestoredAt are the same values the
	// viewmodel.PersonRecord projection would carry; we duplicate
	// them here because the records-layer type is the canonical
	// shape the scan returns.
	return DataQualityIssue{
		SoldierID:  candidate.ID,
		DisplayID:  strings.TrimSpace(candidate.DisplayID),
		Name:       name,
		EntryType:  entryType,
		Group:      group,
		Code:       code,
		Severity:   severity,
		Summary:    summary,
		Detail:     detail,
		ImportPath: candidate.ImportPath,
		RestoredAt: candidate.RestoredAt,
	}
}

func evaluateQualityIssues(candidate qualityScanCandidate, spouseTypes map[int64]string, mode DataQualityMode) []DataQualityIssue {
	issues := make([]DataQualityIssue, 0)
	entryType := normalizeEntryType(candidate.EntryType)
	displayID := strings.TrimSpace(candidate.DisplayID)
	firstName := strings.TrimSpace(candidate.FirstName)
	middleName := strings.TrimSpace(candidate.MiddleName)
	lastName := strings.TrimSpace(candidate.LastName)
	name := buildIssueName(firstName, middleName, lastName)

	// Issue #530: gate Identity & Naming / identity-missing to
	// person-bearing entry types. Event Records (entry_type='event',
	// issue #320) live in the soldiers table but carry no first_name
	// or last_name — their identity surface is kind + begin_date +
	// end_date + linked persons. Without this gate, every event row
	// in any archive with events produces a false positive that
	// floods the review queue. Use the shared helper from
	// internal/models so the entry-type list lives in one place.
	if models.IsPersonBearingEntryType(entryType) {
		if displayID == "" || (firstName == "" && lastName == "") {
			issues = append(issues, candidateIssue(candidate,
				name, entryType, "Identity & Naming", "identity-missing", "high",
				"Core identity data is missing.",
				"Record is missing display ID or both first/last name values.",
			))
		}
	}

	birth, birthErr := dates.ParseCanonical(candidate.BirthDate)
	death, deathErr := dates.ParseCanonical(candidate.DeathDate)
	if strings.TrimSpace(candidate.BirthDate) != "" && birthErr != nil {
		issues = append(issues, candidateIssue(candidate,
			name, entryType, "Dates & Chronology", "birth-date-invalid", "high",
			"Birth date is not in canonical format.",
			fmt.Sprintf("Birth date %q could not be parsed as MM/DD/YYYY with 00 placeholders.", strings.TrimSpace(candidate.BirthDate)),
		))
	}
	if strings.TrimSpace(candidate.DeathDate) != "" && deathErr != nil {
		issues = append(issues, candidateIssue(candidate,
			name, entryType, "Dates & Chronology", "death-date-invalid", "high",
			"Death date is not in canonical format.",
			fmt.Sprintf("Death date %q could not be parsed as MM/DD/YYYY with 00 placeholders.", strings.TrimSpace(candidate.DeathDate)),
		))
	}
	if birthErr == nil && deathErr == nil && chronologyClearlyInvalid(birth, death) {
		issues = append(issues, candidateIssue(candidate,
			name, entryType, "Dates & Chronology", "chronology-death-before-birth", "high",
			"Chronology is contradictory.",
			"Death date is earlier than birth date.",
		))
	}

	if entryType != "soldier" {
		switch {
		case candidate.SpouseSoldierID < 1:
			issues = append(issues, candidateIssue(candidate,
				name, entryType, "Relationship Integrity", "spouse-link-missing", "high",
				"Spouse-linked entry is missing its linked soldier.",
				fmt.Sprintf("Entry type %q requires a spouse_soldier_id.", entryType),
			))
		default:
			spouseType, ok := spouseTypes[candidate.SpouseSoldierID]
			if !ok {
				issues = append(issues, candidateIssue(candidate,
					name, entryType, "Relationship Integrity", "spouse-link-target-missing", "high",
					"Linked spouse target no longer exists.",
					fmt.Sprintf("spouse_soldier_id %d does not match an existing soldier row.", candidate.SpouseSoldierID),
				))
			} else if spouseType != "soldier" {
				issues = append(issues, candidateIssue(candidate,
					name, entryType, "Relationship Integrity", "spouse-link-target-invalid", "high",
					"Linked spouse target is not a soldier record.",
					fmt.Sprintf("spouse_soldier_id %d points to entry type %q.", candidate.SpouseSoldierID, spouseType),
				))
			}
		}
	}

	if hasObviousPlaceholderNoise(firstName, lastName, candidate.BirthInfo, candidate.BuriedIn) {
		issues = append(issues, candidateIssue(candidate,
			name, entryType, "Placeholder Content", "placeholder-noise", "medium",
			"Core fields contain obvious placeholder noise.",
			"Detected obvious placeholder markers (for example lorem/todo/placeholder/asdf/???) in key identity or location fields.",
		))
	}

	// Issue #531: detect raw HTML / unescaped markup noise in the
	// same freeform text fields the placeholder check covers. A
	// Memorial JSON import or a copy-paste from a web page can
	// land markup in Notes / Birth Info / etc. that the static
	// archive then renders as live HTML. Three codes, escalating
	// severity: raw-html-tags (medium), unescaped-entity (medium),
	// mixed-content-script (high — the row carries a script tag
	// or an event-handler attribute, which is a security concern
	// the moment the row is exported to a downloadable archive).
	markup := classifyMarkupNoise(firstName, lastName, candidate.BirthInfo, candidate.BuriedIn)
	if markup != "" {
		severity, summary, detail := markupIssueShape(markup)
		issues = append(issues, candidateIssue(candidate,
			name, entryType, "Field Content", markup, severity, summary, detail,
		))
	}

	if mode == DataQualityModeAdvanced && len(lastName) == 1 {
		issues = append(issues, candidateIssue(candidate,
			name, entryType, "Identity & Naming", "surname-too-short", "medium",
			"Last name looks unusually short.",
			"Last name is a single character; verify this is intentional.",
		))
	}

	return issues
}

func chronologyClearlyInvalid(birth, death dates.PartialDate) bool {
	if birth.Year == 0 || death.Year == 0 {
		return false
	}
	if death.Year < birth.Year {
		return true
	}
	if death.Year > birth.Year {
		return false
	}
	if birth.Month > 0 && death.Month > 0 {
		if death.Month < birth.Month {
			return true
		}
		if death.Month > birth.Month {
			return false
		}
		if birth.Day > 0 && death.Day > 0 && death.Day < birth.Day {
			return true
		}
	}
	return false
}

func hasObviousPlaceholderNoise(values ...string) bool {
	for _, value := range values {
		lower := strings.ToLower(strings.TrimSpace(value))
		if lower == "" {
			continue
		}
		for _, marker := range []string{"lorem ipsum", "placeholder", "todo", "asdf", "???"} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

func buildIssueName(first, middle, last string) string {
	name := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(first),
		strings.TrimSpace(middle),
		strings.TrimSpace(last),
	}, " "))
	if name == "" {
		return "Unnamed Record"
	}
	return strings.Join(strings.Fields(name), " ")
}

// markupNoisePrecedence lists the markup-noise codes (issue #531)
// in severity order. mixed-content-script is the highest severity
// (carries a script tag or an event-handler attribute), then
// raw-html-tags (carries raw markup), then unescaped-entity
// (carries literal &lt; &gt; &amp; that will double-escape on
// re-render). The classifier picks the first match in this order
// so a single field carrying all three noise types surfaces the
// most severe single issue rather than three stacked ones.
var markupNoisePrecedence = []string{
	"mixed-content-script",
	"raw-html-tags",
	"unescaped-entity",
}

// classifyMarkupNoise (issue #531) inspects the freeform text
// fields the placeholder check covers and returns the most severe
// markup-noise code it finds, or "" when the fields are clean.
// The check is intentionally simple — the false-positive cost of
// missing a row that contains raw HTML is higher than the cost
// of catching one that doesn't (per the issue's "simple regex
// sweep is sufficient" guidance).
func classifyMarkupNoise(values ...string) string {
	for _, code := range markupNoisePrecedence {
		if hasMarkupNoiseCode(code, values...) {
			return code
		}
	}
	return ""
}

// hasMarkupNoiseCode returns true when any of the supplied values
// matches the given markup-noise code's pattern. The patterns are
// inline here (not pre-compiled) because the scan is run rarely
// (manual, not in a request hot path) and the regex cache is a
// micro-optimization that adds a package-level init for no real
// gain.
func hasMarkupNoiseCode(code string, values ...string) bool {
	joined := strings.Join(values, "\x00")
	if joined == "" {
		return false
	}
	switch code {
	case "mixed-content-script":
		// script tag, or any event-handler attribute. The
		// attribute pattern matches `on...=` (e.g. onclick=,
		// onerror=) so it catches inline handlers whether they
		// are real HTML attributes or escaped in a markdown
		// snippet.
		return regexpMatch(joined, `(?i)<script\b`) ||
			regexpMatch(joined, `(?i)\son[a-z]+\s*=`)
	case "raw-html-tags":
		// any <tag> or </tag> opener. Self-closing tags are
		// covered by the same `<...>` shape. Balanced OR
		// unbalanced markup both trip this — the user's
		// downstream concern is that the static archive will
		// render the markup as live HTML, not whether the
		// markup is well-formed.
		return regexpMatch(joined, `<[a-zA-Z][a-zA-Z0-9-]*\b`) ||
			regexpMatch(joined, `</[a-zA-Z][a-zA-Z0-9-]*\b`)
	case "unescaped-entity":
		// literal &lt; / &gt; / &amp; that will render as
		// &lt;script&gt; after the next render pass (double-
		// encoded HTML).
		return regexpMatch(joined, `&(lt|gt|amp);`)
	}
	return false
}

// regexpMatch is a thin wrapper around regexp.MatchString that
// keeps the patterns above readable. Panics on a malformed regex
// (the patterns are compile-time constants, so a panic here is a
// developer bug, not a runtime input error).
func regexpMatch(s, pattern string) bool {
	matched, err := regexpMatchCompiled(s, pattern)
	if err != nil {
		// Patterns are compile-time constants; an error here
		// means the developer introduced a bad pattern. Surface
		// the error via a panic rather than silently swallowing
		// it (the scan would otherwise miss every issue the
		// broken pattern was supposed to catch).
		panic(fmt.Sprintf("quality_scan: bad markup-noise regex %q: %v", pattern, err))
	}
	return matched
}

// regexpMatchCompiled is the regexp-backed worker. Exposed via a
// helper so the per-pattern code path above stays readable as a
// list of intent statements rather than a chain of compiled
// regexes.
var regexpMatchCompiled = func(s, pattern string) (bool, error) {
	return regexp.MatchString(pattern, s)
}

// markupIssueShape (issue #531) maps a markup-noise code to the
// (severity, summary, detail) tuple the issue card surfaces. Kept
// as a separate function so the classifier stays a pure
// detector and the user-facing copy can evolve without touching
// the detection logic.
func markupIssueShape(code string) (severity, summary, detail string) {
	switch code {
	case "mixed-content-script":
		return "high",
			"Field carries a script tag or event-handler attribute.",
			"Detected <script> or an on...= attribute. This is a security concern if the row is exported to a static archive — sanitize the field before sharing."
	case "raw-html-tags":
		return "medium",
			"Field carries raw HTML markup.",
			"Detected a <tag> pattern. The static archive will render this as live HTML; strip the markup and re-enter the value as plain text."
	case "unescaped-entity":
		return "medium",
			"Field carries unescaped HTML entities.",
			"Detected &lt; / &gt; / &amp; in a freeform text field. The next render pass will double-escape these — paste the original value (not the rendered HTML) into the field."
	}
	return "medium", "Field carries markup noise.", "Detected markup noise in a freeform text field."
}

// loadEventZeroLinkIssues (issue #320) returns one
// DataQualityIssue per Event Record that has zero
// event_person_links rows. The Event exists but is orphaned
// (no Person Record references it). The Event is still
// reachable via the EVT-NNNNN Display ID, but the researcher
// probably forgot to attach it; the review queue surfaces the
// Event so they can either attach Person Records or delete it.
//
// The Event's user-facing name on a review-queue card is
// composed from kind + begin_date + end_date, not first/last
// name (Events have no Person Record name parts). The Display
// ID still uses the EVT-NNNNN namespace.
func (s *SoldierService) loadEventZeroLinkIssues() ([]DataQualityIssue, error) {
	rows, err := s.db.Conn().Query(
		`SELECT s.id, s.display_id, s.kind, s.begin_date, s.end_date,
		        COALESCE(s.created_by_import_path, ''), COALESCE(s.restored_at, '')
		 FROM soldiers s
		 LEFT JOIN event_person_links epl ON epl.event_id = s.id
		 WHERE s.entry_type = ? AND epl.id IS NULL
		 ORDER BY s.updated_at DESC, s.id DESC`,
		models.EntryTypeEvent,
	)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "loadEventZeroLinkIssues.rows")
	var issues []DataQualityIssue
	for rows.Next() {
		var (
			id          int64
			displayID   string
			kind        string
			beginDate   string
			endDate     string
			importPath  string
			restoredAt  string
		)
		if err := rows.Scan(&id, &displayID, &kind, &beginDate, &endDate, &importPath, &restoredAt); err != nil {
			return nil, err
		}
		name := buildEventIssueName(kind, beginDate, endDate)
		issues = append(issues, DataQualityIssue{
			SoldierID:  id,
			DisplayID:  displayID,
			Name:       name,
			EntryType:  models.EntryTypeEvent,
			Group:      "Event Integrity",
			Code:       "event-zero-links",
			Severity:   "medium",
			Summary:    "Event Record is not linked to any Person Record.",
			Detail:     "Event exists but has zero event_person_links rows. Attach at least one Person Record, or delete the Event if it was created by accident.",
			ImportPath: strings.TrimSpace(importPath),
			RestoredAt: strings.TrimSpace(restoredAt),
		})
	}
	return issues, rows.Err()
}

// buildEventIssueName composes a user-facing label for an Event
// Record on a review-queue card. Pattern: "{kind} ({begin} - {end})"
// with the missing date fields stripped. Falls back to
// "Unnamed Event" when nothing is set.
func buildEventIssueName(kind, beginDate, endDate string) string {
	kind = strings.TrimSpace(kind)
	beginDate = strings.TrimSpace(beginDate)
	endDate = strings.TrimSpace(endDate)
	switch {
	case kind != "" && beginDate != "" && endDate != "":
		return fmt.Sprintf("%s (%s - %s)", kind, beginDate, endDate)
	case kind != "" && beginDate != "":
		return fmt.Sprintf("%s (%s)", kind, beginDate)
	case kind != "" && endDate != "":
		return fmt.Sprintf("%s (- %s)", kind, endDate)
	case kind != "":
		return kind
	case beginDate != "" || endDate != "":
		return fmt.Sprintf("%s - %s", beginDate, endDate)
	default:
		return "Unnamed Event"
	}
}
