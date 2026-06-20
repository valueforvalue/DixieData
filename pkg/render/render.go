// Package render owns the PDF export logic extracted from internal/archive.
// It exposes a Service that renders the existing fpdf-based PDFs; a later
// slice will add a TypstRenderer that renders the same data through Typst
// templates. Slice 0 of the Typst migration plan is this extraction.
package render

import (
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

// UserIdentityStore is the slice of *db.DB the PDF render service needs.
// Defined here so the render package does not import internal/db.
type UserIdentityStore interface {
	UserIdentity() (models.UserIdentity, error)
}

// SoldierLister is the slice of *records.SoldierService the PDF render service
// needs. Defined here for the same reason.
type SoldierLister interface {
	List(page, pageSize int) ([]models.Soldier, int, error)
	GetByID(id int64) (*models.Soldier, error)
}

// AnalyticsSnapshot is re-aliased for callers that don't want to import
// internal/records.
type AnalyticsSnapshot = records.AnalyticsSnapshot

// AnalyticsCount is re-aliased for the same reason.
type AnalyticsCount = records.AnalyticsCount

// Service renders the existing fpdf-based PDF exports. Constructed once
// per ExportService; safe for concurrent calls because fpdf.Fpdf is not
// shared across goroutines -- each top-level method builds its own document.
type Service struct {
	users   UserIdentityStore
	soldier SoldierLister
}

// New constructs a Service. The user identity and soldier service are
// injected as interfaces so this package does not import internal/db or
// internal/records transitively from the import side.
func New(users UserIdentityStore, soldier SoldierLister) *Service {
	return &Service{users: users, soldier: soldier}
}
