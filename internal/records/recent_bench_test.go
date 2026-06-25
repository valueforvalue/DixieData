package records

import (
	"fmt"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// BenchmarkRecentByIDsIsFast is the smoke test for audit issue #119
// (finding 7.2). It seeds N soldiers, asks RecentByIDs for the first
// min(N, 20) IDs, and times the round-trip. The slim
// recentSelectColumns makes RecentByIDs substantially cheaper than
// the previous soldierListSelectColumns (which carried two
// correlated subqueries per row).
func BenchmarkRecentByIDsIsFast(b *testing.B) {
	const seed = 1000
	const want = 20
	d := newTestDB(&testing.T{})
	svc := NewSoldierService(d)

	ids := make([]int64, 0, want)
	for i := 0; i < seed; i++ {
		s, err := svc.Create(models.Soldier{
			DisplayID: fmt.Sprintf("RECENT-%05d", i),
			FirstName: "Recent",
			LastName:  fmt.Sprintf("Bench-%05d", i),
		})
		if err != nil {
			b.Fatalf("seed %d: %v", i, err)
		}
		if i < want {
			ids = append(ids, s.ID)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := svc.RecentByIDs(ids, want)
		if err != nil {
			b.Fatalf("RecentByIDs: %v", err)
		}
		if len(got) != want {
			b.Fatalf("expected %d, got %d", want, len(got))
		}
	}
}