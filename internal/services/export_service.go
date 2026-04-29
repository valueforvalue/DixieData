package services

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

type ExportService struct {
	db      *db.DB
	soldier *SoldierService
}

func NewExportService(database *db.DB, soldier *SoldierService) *ExportService {
	return &ExportService{db: database, soldier: soldier}
}

func (e *ExportService) ExportJSON(outputPath string) error {
	soldiers, _, err := e.soldier.List(1, 100000)
	if err != nil {
		return err
	}

	var full []models.Soldier
	for _, s := range soldiers {
		enriched, err := e.soldier.GetByID(s.ID)
		if err != nil {
			return err
		}
		full = append(full, *enriched)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(full)
}

func (e *ExportService) ExportCSV(outputPath string) error {
	soldiers, _, err := e.soldier.List(1, 100000)
	if err != nil {
		return err
	}

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

	for _, s := range soldiers {
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
	return nil
}
