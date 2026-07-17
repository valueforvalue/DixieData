// Package repo declares the repository interfaces that decouple
// DixieData's domain services from the concrete database driver.
//
// Slice 6 of issue #613 adds the CalendarItemRepo seam.
//
// Calendar items live in the `calendar_items` table (per-month,
// per-day entries the /calendar page renders). Slice 6 covers
// 5 methods: the read paths (GetByID, ListForMonthDay) and the
// write paths (Create, Update, Delete).
package repo

import (
	"context"
	"database/sql"

	"github.com/valueforvalue/DixieData/internal/models"
)

// CalendarItemRepo is the data-access seam for Calendar items.
type CalendarItemRepo interface {
	// Create inserts a new CalendarItem and returns the
	// generated primary-key id.
	Create(ctx context.Context, ex Execer, item models.CalendarItem) (int64, error)

	// GetByID returns the CalendarItem row with the given
	// primary-key ID. Returns sql.ErrNoRows if no row matches.
	GetByID(ctx context.Context, id int64) (*sql.Row, error)

	// ListForMonthDay returns every CalendarItem for the given
	// (month, day) pair, ordered by item_type (holiday < event
	// < other) + title + id. Returns *sql.Rows the caller must
	// Close.
	ListForMonthDay(ctx context.Context, q Querier, month, day int) (*sql.Rows, error)

	// Update modifies the CalendarItem identified by item.ID
	// and returns the rows-affected count.
	Update(ctx context.Context, ex Execer, item models.CalendarItem) (int64, error)

	// Delete removes the CalendarItem identified by id and
	// returns the rows-affected count.
	Delete(ctx context.Context, ex Execer, id int64) (int64, error)
}