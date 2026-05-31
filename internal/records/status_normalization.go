package records

import (
	"database/sql"
	"strings"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
)

const (
	normalizedPensionStateExpr = `CASE
		WHEN LOWER(TRIM(COALESCE(pension_state, ''))) IN ('', 'none', 'na', 'n/a', 'not recorded') THEN 'N/A'
		ELSE TRIM(pension_state)
	END`
	normalizedConfederateHomeStatusExpr = `CASE
		WHEN LOWER(TRIM(COALESCE(confederate_home_status, ''))) IN ('', 'none', 'na', 'n/a', 'not recorded') THEN 'N/A'
		ELSE TRIM(confederate_home_status)
	END`
)

func normalizeOptionalPensionState(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return pensionstate.Normalize(value)
}

func normalizeOptionalConfederateHomeStatus(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return confederatehomestatus.Normalize(value)
}

func distinctNormalizedTextValues(conn *sql.DB, query string, normalize func(string) string) ([]string, error) {
	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := []string{}
	seen := map[string]struct{}{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		normalized := strings.TrimSpace(normalize(value))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, normalized)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
