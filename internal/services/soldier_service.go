package services

import (
	"database/sql"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

type SoldierService struct {
	db *db.DB
}

func NewSoldierService(database *db.DB) *SoldierService {
	return &SoldierService{db: database}
}

func (s *SoldierService) Create(soldier models.Soldier) (*models.Soldier, error) {
	conn := s.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if soldier.DisplayID == "" {
		id, err := s.db.NextCSAID()
		if err != nil {
			return nil, err
		}
		soldier.DisplayID = id
		soldier.IsGenerated = true
	}

	res, err := tx.Exec(`INSERT INTO soldiers (display_id, is_generated, first_name, last_name, rank, unit, death_year, death_month, death_day, birth_info, notes) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		soldier.DisplayID, soldier.IsGenerated, soldier.FirstName, soldier.LastName,
		soldier.Rank, soldier.Unit, soldier.DeathYear, soldier.DeathMonth,
		soldier.DeathDay, soldier.BirthInfo, soldier.Notes)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	soldier.ID = id

	_, err = tx.Exec(`INSERT INTO soldiers_fts(rowid, first_name, last_name, unit, rank) VALUES (?,?,?,?,?)`,
		id, soldier.FirstName, soldier.LastName, soldier.Unit, soldier.Rank)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &soldier, nil
}

func (s *SoldierService) GetByID(id int64) (*models.Soldier, error) {
	conn := s.db.Conn()
	row := conn.QueryRow(`SELECT id, display_id, is_generated, first_name, last_name, rank, unit, death_year, death_month, death_day, birth_info, notes, created_at FROM soldiers WHERE id = ?`, id)
	soldier, err := scanSoldier(row)
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(`SELECT id, soldier_id, record_type, app_id, details FROM records WHERE soldier_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r models.Record
		if err := rows.Scan(&r.ID, &r.SoldierID, &r.RecordType, &r.AppID, &r.Details); err != nil {
			return nil, err
		}
		soldier.Records = append(soldier.Records, r)
	}

	imgRows, err := conn.Query(`SELECT id, soldier_id, file_name, file_path, caption FROM images WHERE soldier_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer imgRows.Close()
	for imgRows.Next() {
		var img models.Image
		if err := imgRows.Scan(&img.ID, &img.SoldierID, &img.FileName, &img.FilePath, &img.Caption); err != nil {
			return nil, err
		}
		soldier.Images = append(soldier.Images, img)
	}

	return soldier, nil
}

func (s *SoldierService) Update(soldier models.Soldier) error {
	conn := s.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE soldiers SET display_id=?, first_name=?, last_name=?, rank=?, unit=?, death_year=?, death_month=?, death_day=?, birth_info=?, notes=? WHERE id=?`,
		soldier.DisplayID, soldier.FirstName, soldier.LastName, soldier.Rank, soldier.Unit,
		soldier.DeathYear, soldier.DeathMonth, soldier.DeathDay, soldier.BirthInfo, soldier.Notes, soldier.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`INSERT INTO soldiers_fts(soldiers_fts, rowid, first_name, last_name, unit, rank) VALUES('delete', ?, ?, ?, ?, ?)`,
		soldier.ID, soldier.FirstName, soldier.LastName, soldier.Unit, soldier.Rank)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO soldiers_fts(rowid, first_name, last_name, unit, rank) VALUES (?,?,?,?,?)`,
		soldier.ID, soldier.FirstName, soldier.LastName, soldier.Unit, soldier.Rank)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SoldierService) Delete(id int64) error {
	_, err := s.db.Conn().Exec(`DELETE FROM soldiers WHERE id = ?`, id)
	return err
}

func (s *SoldierService) Search(query string) ([]models.Soldier, error) {
	conn := s.db.Conn()
	rows, err := conn.Query(`SELECT s.id, s.display_id, s.is_generated, s.first_name, s.last_name, s.rank, s.unit, s.death_year, s.death_month, s.death_day, s.birth_info, s.notes, s.created_at FROM soldiers s JOIN soldiers_fts f ON s.id = f.rowid WHERE soldiers_fts MATCH ? LIMIT 100`, query)
	if err != nil {
		like := "%" + query + "%"
		rows, err = conn.Query(`SELECT id, display_id, is_generated, first_name, last_name, rank, unit, death_year, death_month, death_day, birth_info, notes, created_at FROM soldiers WHERE first_name LIKE ? OR last_name LIKE ? OR unit LIKE ? OR rank LIKE ? LIMIT 100`, like, like, like, like)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()
	return scanSoldiers(rows)
}

func (s *SoldierService) List(page, pageSize int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM soldiers`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := conn.Query(`SELECT id, display_id, is_generated, first_name, last_name, rank, unit, death_year, death_month, death_day, birth_info, notes, created_at FROM soldiers ORDER BY last_name, first_name LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	soldiers, err := scanSoldiers(rows)
	return soldiers, total, err
}

func scanSoldier(row *sql.Row) (*models.Soldier, error) {
	var s models.Soldier
	err := row.Scan(&s.ID, &s.DisplayID, &s.IsGenerated, &s.FirstName, &s.LastName, &s.Rank, &s.Unit, &s.DeathYear, &s.DeathMonth, &s.DeathDay, &s.BirthInfo, &s.Notes, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func scanSoldiers(rows *sql.Rows) ([]models.Soldier, error) {
	var soldiers []models.Soldier
	for rows.Next() {
		var s models.Soldier
		if err := rows.Scan(&s.ID, &s.DisplayID, &s.IsGenerated, &s.FirstName, &s.LastName, &s.Rank, &s.Unit, &s.DeathYear, &s.DeathMonth, &s.DeathDay, &s.BirthInfo, &s.Notes, &s.CreatedAt); err != nil {
			return nil, err
		}
		soldiers = append(soldiers, s)
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, rows.Err()
}
