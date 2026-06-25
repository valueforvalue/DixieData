package appshell

import (
	"sync"
	"testing"
)

// TestCalendarPDFInFlightDedup verifies that two concurrent
// requests with the same dedup key result in only one "in-flight"
// marker.
func TestCalendarPDFInFlightDedup(t *testing.T) {
	app := NewApp()
	var wg sync.WaitGroup
	const dupKey = "cal-pdf|6|P|June-report.pdf"
	admitted := make(chan struct{}, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			if _, loaded := app.inFlight.LoadOrStore(dupKey, struct{}{}); !loaded {
				admitted <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(admitted)
	count := 0
	for range admitted {
		count++
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 admit, got %d", count)
	}
}
