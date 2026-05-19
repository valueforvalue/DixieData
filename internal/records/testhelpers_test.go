package records

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

func configureExportIdentity(t *testing.T, database *db.DB) {
	t.Helper()
	if _, err := database.ConfigureUserIdentity("Samuel", "Thomas", "Carter", 1838); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
}
