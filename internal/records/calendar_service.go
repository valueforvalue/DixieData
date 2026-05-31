package records

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

var ErrCalendarItemNotFound = errors.New("calendar item not found")

type CalendarValidationError struct {
	Message string
}

func (e *CalendarValidationError) Error() string {
	return e.Message
}

type CalendarItemInput struct {
	ItemType string
	Title    string
	Notes    string
}

type CalendarDaySummary struct {
	AnniversaryCount int
	EventCount       int
	HolidayCount     int
}

type CalendarDay struct {
	Month         int
	Day           int
	Items         []models.CalendarItem
	Anniversaries []models.Soldier
}

type CalendarService struct {
	db *db.DB
}

func NewCalendarService(database *db.DB) *CalendarService {
	return &CalendarService{db: database}
}

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
	defer rows.Close()
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

func (c *CalendarService) CreateCalendarItem(month, day int, input CalendarItemInput) (models.CalendarItem, error) {
	if err := validateCalendarDate(month, day); err != nil {
		return models.CalendarItem{}, err
	}
	itemType, title, notes, err := normalizeCalendarItemInput(input)
	if err != nil {
		return models.CalendarItem{}, err
	}
	result, err := c.db.Conn().Exec(`INSERT INTO calendar_items (item_type, month, day, title, notes, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, itemType, month, day, title, notes)
	if err != nil {
		return models.CalendarItem{}, err
	}
	itemID, err := result.LastInsertId()
	if err != nil {
		return models.CalendarItem{}, err
	}
	return c.getCalendarItem(itemID)
}

func (c *CalendarService) UpdateCalendarItem(itemID int64, input CalendarItemInput) (models.CalendarItem, error) {
	if itemID <= 0 {
		return models.CalendarItem{}, &CalendarValidationError{Message: "item_id must be greater than 0"}
	}
	itemType, title, notes, err := normalizeCalendarItemInput(input)
	if err != nil {
		return models.CalendarItem{}, err
	}
	result, err := c.db.Conn().Exec(`UPDATE calendar_items SET item_type = ?, title = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, itemType, title, notes, itemID)
	if err != nil {
		return models.CalendarItem{}, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return models.CalendarItem{}, err
	}
	if updated == 0 {
		return models.CalendarItem{}, ErrCalendarItemNotFound
	}
	return c.getCalendarItem(itemID)
}

func (c *CalendarService) DeleteCalendarItem(itemID int64) error {
	if itemID <= 0 {
		return &CalendarValidationError{Message: "item_id must be greater than 0"}
	}
	result, err := c.db.Conn().Exec(`DELETE FROM calendar_items WHERE id = ?`, itemID)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
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
	rows, err := c.db.Conn().Query(`SELECT id, item_type, month, day, title, notes, created_at, updated_at
		FROM calendar_items
		WHERE month = ? AND day = ?
		ORDER BY CASE item_type WHEN 'holiday' THEN 0 WHEN 'event' THEN 1 ELSE 2 END, LOWER(title), id`, month, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
	item, err := scanCalendarItem(c.db.Conn().QueryRow(`SELECT id, item_type, month, day, title, notes, created_at, updated_at FROM calendar_items WHERE id = ?`, itemID))
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
