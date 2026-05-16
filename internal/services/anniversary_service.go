package services

import (
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

type AnniversaryService struct {
	db *db.DB
}

func NewAnniversaryService(database *db.DB) *AnniversaryService {
	return &AnniversaryService{db: database}
}

func (a *AnniversaryService) GetByMonthDay(month, day int) ([]models.Soldier, error) {
	conn := a.db.Conn()

	var query string
	var args []interface{}
	if day == 0 {
		query = `SELECT ` + soldierSelectColumns + ` FROM soldiers WHERE death_month = ? AND death_day = 0`
		args = []interface{}{month}
	} else {
		query = `SELECT ` + soldierSelectColumns + ` FROM soldiers WHERE death_month = ? AND death_day = ?`
		args = []interface{}{month, day}
	}

	r, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return scanSoldiers(r)
}

func (a *AnniversaryService) GetMonthCalendar(month int) (map[int][]models.Soldier, error) {
	result := make(map[int][]models.Soldier)
	for day := 0; day <= 31; day++ {
		soldiers, err := a.GetByMonthDay(month, day)
		if err != nil {
			return nil, err
		}
		if len(soldiers) > 0 {
			result[day] = soldiers
		}
	}
	return result, nil
}
