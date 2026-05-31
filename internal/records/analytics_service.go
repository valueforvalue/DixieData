package records

import (
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

type AnalyticsService struct {
	db *db.DB
}

type AnalyticsCount struct {
	Label string
	Count int
}

type AnalyticsSnapshot struct {
	RecordTypes             models.ArchiveCounts
	CemeteryDensity         []AnalyticsCount
	ConfederateHomeStatus   []AnalyticsCount
	ConfederateHomeNames    []AnalyticsCount
	PensionDistribution     []AnalyticsCount
	UnitRepresentation      []AnalyticsCount
	BirthDecadeDistribution []AnalyticsCount
	DeathDecadeDistribution []AnalyticsCount
	DuplicateAudit          DuplicateAuditSummary
}

func NewAnalyticsService(database *db.DB) *AnalyticsService {
	return &AnalyticsService{db: database}
}

func (s *AnalyticsService) Snapshot() (AnalyticsSnapshot, error) {
	recordTypes, err := NewSoldierService(s.db).ArchiveCounts()
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	cemeteries, err := s.queryCounts(`
		SELECT TRIM(buried_in) AS label, COUNT(*)
		FROM soldiers
		WHERE TRIM(COALESCE(buried_in, '')) <> ''
		GROUP BY TRIM(buried_in)
		ORDER BY COUNT(*) DESC, LOWER(TRIM(buried_in)) ASC
		LIMIT 10`)
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	homeStatuses, err := s.queryCounts(`
		SELECT CASE
			WHEN LOWER(TRIM(COALESCE(confederate_home_status, ''))) IN ('', 'none', 'na', 'n/a') THEN 'NA'
			ELSE TRIM(confederate_home_status)
		END AS label, COUNT(*)
		FROM soldiers
		GROUP BY label
		ORDER BY COUNT(*) DESC, LOWER(label) ASC`)
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	for i := range homeStatuses {
		homeStatuses[i].Label = confederatehomestatus.Normalize(homeStatuses[i].Label)
	}
	homeNames, err := s.queryCounts(`
		SELECT TRIM(confederate_home_name) AS label, COUNT(*)
		FROM soldiers
		WHERE TRIM(COALESCE(confederate_home_name, '')) <> ''
		GROUP BY TRIM(confederate_home_name)
		ORDER BY COUNT(*) DESC, LOWER(TRIM(confederate_home_name)) ASC
		LIMIT 5`)
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	pensionStates, err := s.queryCounts(`
		SELECT TRIM(pension_state) AS label, COUNT(*)
		FROM soldiers
		WHERE TRIM(COALESCE(pension_state, '')) <> ''
		GROUP BY TRIM(pension_state)
		ORDER BY COUNT(*) DESC, LOWER(TRIM(pension_state)) ASC`)
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	units, err := s.queryCounts(`
		SELECT TRIM(unit) AS label, COUNT(*)
		FROM soldiers
		WHERE TRIM(COALESCE(unit, '')) <> ''
		GROUP BY TRIM(unit)
		ORDER BY COUNT(*) DESC, LOWER(TRIM(unit)) ASC
		LIMIT 5`)
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	birthDecades, err := s.queryDecadeCounts(`
		SELECT ((CAST(SUBSTR(birth_date, 7, 4) AS INTEGER) / 10) * 10) AS decade, COUNT(*)
		FROM soldiers
		WHERE LENGTH(TRIM(COALESCE(birth_date, ''))) >= 10
		  AND CAST(SUBSTR(birth_date, 7, 4) AS INTEGER) >= 1000
		GROUP BY decade
		ORDER BY decade ASC`)
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	deathDecades, err := s.queryDecadeCounts(`
		SELECT ((death_year / 10) * 10) AS decade, COUNT(*)
		FROM soldiers
		WHERE death_year >= 1000
		GROUP BY decade
		ORDER BY decade ASC`)
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	duplicateAudit, err := NewAuditService(s.db).Summary()
	if err != nil {
		return AnalyticsSnapshot{}, err
	}
	return AnalyticsSnapshot{
		RecordTypes:             recordTypes,
		CemeteryDensity:         cemeteries,
		ConfederateHomeStatus:   homeStatuses,
		ConfederateHomeNames:    homeNames,
		PensionDistribution:     pensionStates,
		UnitRepresentation:      units,
		BirthDecadeDistribution: birthDecades,
		DeathDecadeDistribution: deathDecades,
		DuplicateAudit:          duplicateAudit,
	}, nil
}

func (s *AnalyticsService) queryCounts(query string, args ...any) ([]AnalyticsCount, error) {
	rows, err := s.db.Conn().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []AnalyticsCount{}
	for rows.Next() {
		var label string
		var count int
		if err := rows.Scan(&label, &count); err != nil {
			return nil, err
		}
		label = strings.TrimSpace(label)
		if label == "" {
			label = "Not Recorded"
		}
		results = append(results, AnalyticsCount{Label: label, Count: count})
	}
	return results, rows.Err()
}

func (s *AnalyticsService) queryDecadeCounts(query string, args ...any) ([]AnalyticsCount, error) {
	rows, err := s.db.Conn().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []AnalyticsCount{}
	for rows.Next() {
		var decade int
		var count int
		if err := rows.Scan(&decade, &count); err != nil {
			return nil, err
		}
		if decade < 1000 {
			continue
		}
		results = append(results, AnalyticsCount{
			Label: fmt.Sprintf("%ds", decade),
			Count: count,
		})
	}
	return results, rows.Err()
}
