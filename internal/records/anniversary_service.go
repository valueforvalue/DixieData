package records

import (
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

// AnniversaryService owns the per-soldier "on this day" + "this
// month" anniversary computation that drives the calendar page +
// the iCal export. Constructed by NewAnniversaryService and held
// by *App. Anniversary dates are stored as month/day only (no
// year) so the service is tz-naive for date math; CalendarTimeZone
// in internal/buildinfo governs only "today" semantics.
type AnniversaryService struct {
	db *db.DB
}

// NewAnniversaryService constructs an AnniversaryService bound to
// the given database.
func NewAnniversaryService(database *db.DB) *AnniversaryService {
	return &AnniversaryService{db: database}
}

// GetByMonthDay returns the per-soldier anniversaries that match the given month + day.
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

// GetMonthCalendar returns the calendar page payload for the given month: per-day summaries + per-day details.
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
