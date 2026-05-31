package records

import (
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/models"
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
	if err := conn.QueryRow(`SELECT COUNT(*) FROM soldiers`+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, request, err
	}

	rows, err := conn.Query(
		fmt.Sprintf(`SELECT %s FROM soldiers%s ORDER BY %s LIMIT ? OFFSET ?`, soldierListSelectColumns, whereClause, orderBy),
		append(args, request.PageSize, offset)...,
	)
	if err != nil {
		return nil, 0, request, err
	}
	defer rows.Close()

	soldiers, err := scanListSoldiers(rows)
	return soldiers, total, request, err
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
