package appshell

import "github.com/valueforvalue/DixieData/internal/models"

// Type aliases for the App service surface (issue #343
// finding #4 — facade interfaces deleted; aliases moved here
// from app_facades.go so callers that already use the short
// name keep compiling).
type (
	personRecord            = models.Soldier
	personRecordSearch      = models.SoldierSearch
	personRecordSuggestions = models.SoldierFormSuggestions
)
