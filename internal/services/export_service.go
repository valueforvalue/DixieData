package services

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/valueforvalue/DixieData/internal/db"
)

const exportBatchSize = 500

type ExportService struct {
	db      *db.DB
	soldier *SoldierService
}

func NewExportService(database *db.DB, soldier *SoldierService) *ExportService {
	return &ExportService{db: database, soldier: soldier}
}

// ExportJSON streams a full hierarchical export (Soldier -> Records -> Images)
// to outputPath in JSON array format, processing records in batches to avoid
// loading the entire dataset into memory at once.
func (e *ExportService) ExportJSON(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	if _, err := fmt.Fprint(f, "[\n"); err != nil {
		return err
	}

	first := true
	page := 1
	for {
		batch, _, err := e.soldier.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		for _, s := range batch {
			enriched, err := e.soldier.GetByID(s.ID)
			if err != nil {
				return err
			}
			if !first {
				if _, err := fmt.Fprint(f, ",\n"); err != nil {
					return err
				}
			}
			if err := enc.Encode(enriched); err != nil {
				return err
			}
			first = false
		}

		if len(batch) < exportBatchSize {
			break
		}
		page++
	}

	_, err = fmt.Fprint(f, "]\n")
	return err
}

// ExportCSV streams a flat CSV export of all soldiers, processing records in
// batches to avoid loading the entire dataset into memory at once.
func (e *ExportService) ExportCSV(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"id", "display_id", "is_generated", "first_name", "last_name", "rank", "unit", "death_year", "death_month", "death_day", "birth_info", "notes", "created_at"}
	if err := w.Write(header); err != nil {
		return err
	}

	page := 1
	for {
		batch, _, err := e.soldier.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		for _, s := range batch {
			row := []string{
				fmt.Sprintf("%d", s.ID),
				s.DisplayID,
				fmt.Sprintf("%v", s.IsGenerated),
				s.FirstName,
				s.LastName,
				s.Rank,
				s.Unit,
				fmt.Sprintf("%d", s.DeathYear),
				fmt.Sprintf("%d", s.DeathMonth),
				fmt.Sprintf("%d", s.DeathDay),
				s.BirthInfo,
				s.Notes,
				s.CreatedAt,
			}
			if err := w.Write(row); err != nil {
				return err
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return err
		}

		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	return nil
}
