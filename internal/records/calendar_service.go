package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
	sqliterepo "github.com/valueforvalue/DixieData/internal/db/repo/sqlite"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/models"
)

var ErrCalendarItemNotFound = errors.New("calendar item not found")

// CalendarValidationError is a records-layer type used by the matching service.
type CalendarValidationError struct {
	Message string
}

// Error is the records-layer method backing the matching viewmodel mapper.
func (e *CalendarValidationError) Error() string {
	return e.Message
}

// CalendarItemInput is a records-layer type used by the matching service.
type CalendarItemInput struct {
	ItemType string
	Title    string
	Notes    string
}

// CalendarDaySummary is a records-layer type used by the matching service.
type CalendarDaySummary struct {
	AnniversaryCount int
	EventCount       int
	HolidayCount     int
}

// CalendarDay is a records-layer type used by the matching service.
type CalendarDay struct {
	Month         int
	Day           int
	Items         []models.CalendarItem
	Anniversaries []models.Soldier
}

// CalendarService produces the per-month grid the calendar page
// renders: one row per day, one cell per soldier with an
// anniversary that day. Owns the per-month PDF dispatch (one
// download endpoint per month). Constructed by NewCalendarService.
type CalendarService struct {
	db        *db.DB
	calItemRepo repo.CalendarItemRepo
}

// NewCalendarService constructs a CalendarService bound to the
// given database.
func NewCalendarService(database *db.DB) *CalendarService {
	return &CalendarService{
		db:          database,
		calItemRepo: sqliterepo.NewCalendarItemRepo(database),
	}
}

// GetMonthSummary returns the per-month grid the calendar page renders: one CalendarDaySummary per day in the month.
func (c *CalendarService) GetMonthSummary(month int) (map[int]CalendarDaySummary, error) {
	if err := validateCalendarMonth(month); err != nil {
		return nil, err
	}
	conn := c.db.Conn()
	result := make(map[int]CalendarDaySummary)

	rows, err := conn.Query(`SELECT death_day, COUNT(*) FROM soldiers WHERE death_month = ? AND death_day BETWEEN 1 AND 31 GROUP BY death_day`, month)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var day int
		var count int
		if err := rows.Scan(&day, &count); err != nil {
			return nil, err
		}
		summary := result[day]
		summary.AnniversaryCount = count
		result[day] = summary
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = conn.Query(`SELECT day, item_type, COUNT(*) FROM calendar_items WHERE month = ? GROUP BY day, item_type`, month)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "GetMonthSummary.rows")
	for rows.Next() {
		var day int
		var itemType string
		var count int
		if err := rows.Scan(&day, &itemType, &count); err != nil {
			return nil, err
		}
		summary := result[day]
		switch itemType {
		case models.CalendarItemTypeHoliday:
			summary.HolidayCount = count
		case models.CalendarItemTypeEvent:
			summary.EventCount = count
		}
		result[day] = summary
	}
	return result, rows.Err()
}

// GetDay returns the per-day detail panel for the given date: full list of anniversaries + per-item 'open Person Record' links.
func (c *CalendarService) GetDay(month, day int) (CalendarDay, error) {
	if err := validateCalendarMonth(month); err != nil {
		return CalendarDay{}, err
	}
	if day < 0 || day > 31 {
		return CalendarDay{}, &CalendarValidationError{Message: "day must be between 0 and 31"}
	}
	anniversaries, err := NewAnniversaryService(c.db).GetByMonthDay(month, day)
	if err != nil {
		return CalendarDay{}, err
	}
	items, err := c.listCalendarItems(month, day)
	if err != nil {
		return CalendarDay{}, err
	}
	return CalendarDay{
		Month:        month,
		Day:          day,
		Items:        items,
		Anniversaries: anniversaries,
	}, nil
}

// CreateCalendarItem persists a new calendar entry attached to a Soldier.
func (c *CalendarService) CreateCalendarItem(month, day int, input CalendarItemInput) (models.CalendarItem, error) {
	if err := validateCalendarDate(month, day); err != nil {
		return models.CalendarItem{}, err
	}
	itemType, title, notes, err := normalizeCalendarItemInput(input)
	if err != nil {
		return models.CalendarItem{}, err
	}
	// Slice 6 of issue #613: the INSERT goes through the
	// CalendarItemRepo seam. The legacy used
	// CURRENT_TIMESTAMP in the SQL string; the parameterized
	// repo path stamps updated_at in Go with the same
	// timestamp the schema would have generated.
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	item := models.CalendarItem{
		ItemType:  itemType,
		Month:     month,
		Day:       day,
		Title:     title,
		Notes:     notes,
		UpdatedAt: now,
	}
	itemID, err := c.calItemRepo.Create(context.Background(), c.db.Conn(), item)
	if err != nil {
		return models.CalendarItem{}, err
	}
	return c.getCalendarItem(itemID)
}

// UpdateCalendarItem updates an existing calendar entry's date / label / source-record linkage.
func (c *CalendarService) UpdateCalendarItem(itemID int64, input CalendarItemInput) (models.CalendarItem, error) {
	if itemID <= 0 {
		return models.CalendarItem{}, &CalendarValidationError{Message: "item_id must be greater than 0"}
	}
	itemType, title, notes, err := normalizeCalendarItemInput(input)
	if err != nil {
		return models.CalendarItem{}, err
	}
	// Slice 6 of issue #613: the UPDATE goes through the
	// CalendarItemRepo seam. The legacy used
	// CURRENT_TIMESTAMP; the parameterized repo path stamps
	// updated_at in Go.
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	item := models.CalendarItem{
		ID:       itemID,
		ItemType: itemType,
		Title:    title,
		Notes:    notes,
		UpdatedAt: now,
	}
	updated, err := c.calItemRepo.Update(context.Background(), c.db.Conn(), item)
	if err != nil {
		return models.CalendarItem{}, err
	}
	if updated == 0 {
		return models.CalendarItem{}, ErrCalendarItemNotFound
	}
	return c.getCalendarItem(itemID)
}

// DeleteCalendarItem removes a calendar entry.
func (c *CalendarService) DeleteCalendarItem(itemID int64) error {
	if itemID <= 0 {
		return &CalendarValidationError{Message: "item_id must be greater than 0"}
	}
	// Slice 6 of issue #613: the DELETE goes through the
	// CalendarItemRepo seam.
	deleted, err := c.calItemRepo.Delete(context.Background(), c.db.Conn(), itemID)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return ErrCalendarItemNotFound
	}
	return nil
}

func (c *CalendarService) listCalendarItems(month, day int) ([]models.CalendarItem, error) {
	if day < 1 || day > 31 {
		return nil, nil
	}
	// Slice 6 of issue #613: the SELECT goes through the
	// CalendarItemRepo seam.
	rows, err := c.calItemRepo.ListForMonthDay(context.Background(), c.db.Conn(), month, day)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "listCalendarItems.rows")
	var items []models.CalendarItem
	for rows.Next() {
		item, err := scanCalendarItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *CalendarService) getCalendarItem(itemID int64) (models.CalendarItem, error) {
	// Slice 6 of issue #613: the SELECT goes through the
	// CalendarItemRepo seam.
	row, err := c.calItemRepo.GetByID(context.Background(), itemID)
	if err != nil {
		return models.CalendarItem{}, err
	}
	item, err := scanCalendarItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.CalendarItem{}, ErrCalendarItemNotFound
	}
	return item, err
}

func scanCalendarItem(scanner interface{ Scan(dest ...any) error }) (models.CalendarItem, error) {
	var item models.CalendarItem
	if err := scanner.Scan(
		&item.ID,
		&item.ItemType,
		&item.Month,
		&item.Day,
		&item.Title,
		&item.Notes,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.CalendarItem{}, err
	}
	return item, nil
}

func normalizeCalendarItemInput(input CalendarItemInput) (string, string, string, error) {
	itemType := strings.ToLower(strings.TrimSpace(input.ItemType))
	title := strings.TrimSpace(input.Title)
	notes := strings.TrimSpace(input.Notes)
	switch itemType {
	case models.CalendarItemTypeEvent, models.CalendarItemTypeHoliday:
	default:
		return "", "", "", &CalendarValidationError{Message: fmt.Sprintf("item_type must be %q or %q", models.CalendarItemTypeEvent, models.CalendarItemTypeHoliday)}
	}
	if title == "" {
		return "", "", "", &CalendarValidationError{Message: "title is required"}
	}
	return itemType, title, notes, nil
}

func validateCalendarMonth(month int) error {
	if month < 1 || month > 12 {
		return &CalendarValidationError{Message: "month must be between 1 and 12"}
	}
	return nil
}

func validateCalendarDate(month, day int) error {
	if err := validateCalendarMonth(month); err != nil {
		return err
	}
	if day < 1 || day > 31 {
		return &CalendarValidationError{Message: "day must be between 1 and 31"}
	}
	return nil
}
