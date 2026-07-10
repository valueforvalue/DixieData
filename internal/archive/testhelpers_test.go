package archive

import (
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(testtemp.New(t).Path())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
