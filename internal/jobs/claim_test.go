// claim_test.go — issue #615 slice 1 regression net.

package jobs

import (
	"sync"
	"testing"
)

// TestTryClaimAcquiresAndReleases verifies the happy path: one
// caller acquires a claim, releases it, and a second caller can
// then acquire the same key.
func TestTryClaimAcquiresAndReleases(t *testing.T) {
	reg := New()

	release, ok := reg.TryClaim("test:single")
	if !ok {
		t.Fatal("first TryClaim must succeed")
	}
	if release == nil {
		t.Fatal("release must be non-nil on success")
	}

	// Second call with same key while first is held.
	_, ok2 := reg.TryClaim("test:single")
	if ok2 {
		t.Fatal("second TryClaim with same key must return false while held")
	}

	// Release and re-acquire.
	release()
	release3, ok3 := reg.TryClaim("test:single")
	if !ok3 {
		t.Fatal("third TryClaim after release must succeed")
	}
	release3()
}

// TestTryClaimDifferentKeysProceedIndependently verifies that two
// callers with different keys both succeed.
func TestTryClaimDifferentKeysProceedIndependently(t *testing.T) {
	reg := New()

	r1, ok1 := reg.TryClaim("test:a")
	if !ok1 {
		t.Fatal("first key must succeed")
	}
	defer r1()

	r2, ok2 := reg.TryClaim("test:b")
	if !ok2 {
		t.Fatal("different key must succeed concurrently")
	}
	r2()
}

// TestTryClaimReleaseIsIdempotent verifies calling release multiple
// times does not panic or corrupt state.
func TestTryClaimReleaseIsIdempotent(t *testing.T) {
	reg := New()

	release, ok := reg.TryClaim("test:idempotent")
	if !ok {
		t.Fatal("TryClaim must succeed")
	}
	release()
	release() // second call must not panic
	release() // third call must not panic

	// After release, re-acquire must still work.
	r2, ok2 := reg.TryClaim("test:idempotent")
	if !ok2 {
		t.Fatal("re-acquire after idempotent release must succeed")
	}
	r2()
}

// TestTryClaimRace verifies N concurrent callers for the same key
// produce exactly one success.
func TestTryClaimRace(t *testing.T) {
	reg := New()
	const N = 10

	var wg sync.WaitGroup
	successes := make(chan bool, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok := reg.TryClaim("test:race")
			successes <- ok
		}()
	}
	wg.Wait()
	close(successes)

	got := 0
	for s := range successes {
		if s {
			got++
		}
	}
	if got != 1 {
		t.Errorf("expected exactly 1 success in %d-way race; got %d", N, got)
	}
}

// TestTryClaimZeroValueRegistry verifies a nil claims map (zero-value
// sync.Map) works — no explicit init needed. This guards against a
// future refactor that adds an init step and forgets to wire it.
func TestTryClaimZeroValueRegistry(t *testing.T) {
	reg := New() // New initializes claims as zero-value sync.Map
	release, ok := reg.TryClaim("test:zero")
	if !ok {
		t.Fatal("zero-value claims map must work")
	}
	release()
}
