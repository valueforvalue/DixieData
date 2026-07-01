package records

import (
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
)

const (
	BrowseScopeAll           = "all"
	BrowseScopeRecentlyAdded = "recently_added"
	BrowseScopeLastImport    = "last_import"

	BrowseSortDisplayIDAsc   = "display_id_asc"
	BrowseSortDisplayIDDesc  = "display_id_desc"
	BrowseSortNameAsc        = "name_asc"
	BrowseSortLastEditedDesc = "last_edited_desc"
	BrowseSortCreatedDesc    = "created_desc"

	defaultBrowsePageSize = 100
	maxBrowsePageSize     = 250
)

type BrowseRequest struct {
	Page                  int
	PageSize              int
	Scope                 string
	Sort                  string
	EntryType             string
	Unit                  string
	BuriedIn              string
	PensionState          string
	ReviewStatus          string
	ConfederateHomeStatus string
	Tags                  []string
}

func normalizeBrowseRequest(request BrowseRequest) BrowseRequest {
	if request.Page < 1 {
		request.Page = 1
	}
	if request.PageSize < 1 {
		request.PageSize = defaultBrowsePageSize
	}
	if request.PageSize > maxBrowsePageSize {
		request.PageSize = maxBrowsePageSize
	}
	request.Scope = strings.TrimSpace(strings.ToLower(request.Scope))
	switch request.Scope {
	case BrowseScopeRecentlyAdded, BrowseScopeLastImport:
	default:
		request.Scope = BrowseScopeAll
	}
	request.Sort = strings.TrimSpace(strings.ToLower(request.Sort))
	switch request.Sort {
	case BrowseSortDisplayIDDesc, BrowseSortNameAsc, BrowseSortLastEditedDesc, BrowseSortCreatedDesc:
	default:
		request.Sort = BrowseSortDisplayIDAsc
	}
	request.EntryType = strings.TrimSpace(strings.ToLower(request.EntryType))
	request.Unit = strings.TrimSpace(request.Unit)
	request.BuriedIn = strings.TrimSpace(request.BuriedIn)
	request.PensionState = normalizeOptionalPensionState(request.PensionState)
	request.ReviewStatus = strings.TrimSpace(strings.ToLower(request.ReviewStatus))
	switch request.ReviewStatus {
	case "", "clean", "review":
	default:
		request.ReviewStatus = ""
	}
	request.ConfederateHomeStatus = normalizeOptionalConfederateHomeStatus(request.ConfederateHomeStatus)
	// Tag filter: trim + dedupe + drop blanks; the SQL adds the
	// AND-logic HAVING clause because normalised-name collisions are
	// prevented by the tags.normalized_name UNIQUE column.
	cleaned := make([]string, 0, len(request.Tags))
	seen := map[string]bool{}
	for _, t := range request.Tags {
		normalized := NormalizeTagName(t)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		cleaned = append(cleaned, normalized)
	}
	request.Tags = cleaned
	return request
}

func (s *SoldierService) BrowsePage(request BrowseRequest) ([]models.Soldier, int, BrowseRequest, error) {
	request = normalizeBrowseRequest(request)

	whereParts := []string{}
	args := []interface{}{}

	switch request.Scope {
	case BrowseScopeRecentlyAdded:
		whereParts = append(whereParts, `created_at IS NOT NULL AND TRIM(created_at) != ''`)
	case BrowseScopeLastImport:
		whereParts = append(whereParts, `import_batch_id = (SELECT id FROM import_batches ORDER BY created_at DESC, id DESC LIMIT 1)`)
	}

	if request.EntryType != "" {
		whereParts = append(whereParts, `LOWER(TRIM(entry_type)) = ?`)
		args = append(args, request.EntryType)
	}
	if request.Unit != "" {
		whereParts = append(whereParts, `TRIM(unit) = ?`)
		args = append(args, request.Unit)
	}
	if request.BuriedIn != "" {
		whereParts = append(whereParts, `TRIM(buried_in) = ?`)
		args = append(args, request.BuriedIn)
	}
	if request.PensionState != "" {
		whereParts = append(whereParts, normalizedPensionStateExpr+` = ?`)
		args = append(args, request.PensionState)
	}
	switch request.ReviewStatus {
	case "clean":
		whereParts = append(whereParts, `needs_review = 0`)
	case "review":
		whereParts = append(whereParts, `needs_review = 1`)
	}
	if request.ConfederateHomeStatus != "" {
		whereParts = append(whereParts, normalizedConfederateHomeStatusExpr+` = ?`)
		args = append(args, request.ConfederateHomeStatus)
	}

	// Tag AND-filter: Build a HAVING clause that counts distinct tag
	// ids present on the soldier's binding rows so a query for 3
	// tags returns only Person Records that have all three attached.
	// The HAVING is applied to the CTE filtered set so it composes
	// with every WHERE filter above; a soldier with 0 matching tags
	// never enters the count aggregation, dropping them. Tags with
	// no resolved row in tags (legacy / typo) are dropped silently
	// here via NOT IN (id NULL) — the HAVING count then never sees
	// them, so the filter behaves like an ordinary existential.
	//
	// See internal/records/browse_filter_test.go (issue #183).
	if len(request.Tags) > 0 {
		tagPlaceholders := make([]string, len(request.Tags))
		tagArgs := make([]interface{}, len(request.Tags))
		for i, name := range request.Tags {
			tagPlaceholders[i] = "?"
			tagArgs[i] = name
		}
		subquery := fmt.Sprintf(
			`id IN (
				SELECT prt.person_id
				FROM person_record_tags prt
				JOIN tags t ON t.id = prt.tag_id
				WHERE t.normalized_name IN (%s)
				GROUP BY prt.person_id
				HAVING COUNT(DISTINCT t.id) = ?
			)`, strings.Join(tagPlaceholders, ","))
		whereParts = append(whereParts, subquery)
		args = append(args, tagArgs...)
		args = append(args, len(request.Tags))
	}

	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	orderBy := browseOrderClause(request.Sort)
	offset := (request.Page - 1) * request.PageSize
	conn := s.db.Conn()

	var total int
	// Combine the COUNT and the paginated SELECT into a single CTE so
	// every browse filter change costs one round-trip instead of two.
	// The window function COUNT(*) OVER () returns the same value for
	// every row in the filtered set. Audit issue #107 (7.12).
	//
	// The CTE materialises the filtered set with the spouse-link subquery
	// resolved against the soldiers table. The outer SELECT re-uses the
	// CTE columns directly (no soldiers. qualifier) so SQLite resolves
	// every column against the CTE rowset.
	query := fmt.Sprintf(
		`WITH filtered AS (
			SELECT %s, COUNT(*) OVER () AS total_count
			FROM soldiers%s
		)
		SELECT *
		FROM filtered
		ORDER BY %s
		LIMIT ? OFFSET ?`,
		soldierListSelectColumns, whereClause, orderBy,
	)
	rows, err := conn.Query(query, append(args, request.PageSize, offset)...)
	if err != nil {
		return nil, 0, request, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, 0, request, err
	}
	totalIdx := -1
	for i, name := range columns {
		if name == "total_count" {
			totalIdx = i
			break
		}
	}
	if totalIdx < 0 {
		return nil, 0, request, fmt.Errorf("browse CTE missing total_count column")
	}

	scanCols := len(columns) - 1
	var soldiers []models.Soldier
	for rows.Next() {
		var s models.Soldier
		dests := make([]interface{}, scanCols)
		for i := 0; i < scanCols; i++ {
			dests[i] = scanHolder(soldierListScanDest(&s), i)
		}
		if err := rows.Scan(append(dests, &total)...); err != nil {
			return nil, 0, request, err
		}
		hydrateLegacyDeathParts(&s)
		s.PensionState = pensionstate.Normalize(s.PensionState)
		normalizeConfederateHomeFields(&s)
		soldiers = append(soldiers, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, request, err
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, total, request, nil
}

// scanHolder returns a pointer-shaped holder for the i-th destination in
// the soldier list scan dest slice. SQLite hands us a generic dest array
// because the CTE adds a trailing total_count column.
func scanHolder(dests []interface{}, i int) interface{} {
	if i >= len(dests) {
		return new(interface{})
	}
	return dests[i]
}

func browseOrderClause(sort string) string {
	displayIDSequenceExpr := `CASE
		WHEN LENGTH(TRIM(display_id)) >= 5
			AND SUBSTR(TRIM(display_id), LENGTH(TRIM(display_id)) - 4, 5) GLOB '[0-9][0-9][0-9][0-9][0-9]'
		THEN CAST(SUBSTR(TRIM(display_id), LENGTH(TRIM(display_id)) - 4, 5) AS INTEGER)
		ELSE 2147483647
	END`
	switch sort {
	case BrowseSortDisplayIDDesc:
		return displayIDSequenceExpr + ` DESC, UPPER(TRIM(display_id)) DESC, id DESC`
	case BrowseSortNameAsc:
		return `LOWER(TRIM(last_name)) ASC, LOWER(TRIM(first_name)) ASC, LOWER(TRIM(middle_name)) ASC, id ASC`
	case BrowseSortLastEditedDesc:
		return `COALESCE(NULLIF(last_edited_at, ''), NULLIF(updated_at, ''), NULLIF(created_at, '')) DESC, id DESC`
	case BrowseSortCreatedDesc:
		return `COALESCE(NULLIF(created_at, ''), NULLIF(updated_at, '')) DESC, id DESC`
	default:
		return displayIDSequenceExpr + ` ASC, UPPER(TRIM(display_id)) ASC, id ASC`
	}
}
