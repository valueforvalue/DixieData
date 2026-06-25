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
