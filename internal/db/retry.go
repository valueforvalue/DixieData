package db

import (
	"errors"
	"strings"
	"time"
)

// IsBusyError reports whether err looks like a SQLite SQLITE_BUSY (5)
// or SQLITE_LOCKED (6) error. Modernc sqlite returns the driver-level
// error string ("database is locked (5) (SQLITE_BUSY)") rather than a
// typed sentinel, so we match on the canonical message fragments.
func IsBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") ||
		strings.Contains(msg, "SQLITE_LOCKED") ||
		strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked")
}

// WithBusyRetry runs fn and, if it returns a SQLITE_BUSY/LOCKED error,
// retries up to maxAttempts with a brief backoff. The default pragma
// `busy_timeout(5000)` covers most cases, but pooled connections have
// been observed to occasionally miss the pragma on first use, surfacing
// as a fast-fail SQLITE_BUSY under concurrent reads. This helper makes
// the read paths tolerant of those races without changing the storage
// layer.
func WithBusyRetry(maxAttempts int, fn func() error) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil || !IsBusyError(err) {
			return err
		}
		if attempt == maxAttempts {
			break
		}
		// 5ms, 25ms, 125ms — capped exponential. Long enough to let a
		// committing writer release the WAL writer frame, short enough
		// that a request handler doesn't noticeably stall.
		time.Sleep(time.Duration(5*pow5(attempt-1)) * time.Millisecond)
	}
	return err
}

// pow5 returns 5^exp for small non-negative exp. Inline to avoid the
// math package dependency in this hot path.
func pow5(exp int) int {
	v := 1
	for i := 0; i < exp; i++ {
		v *= 5
	}
	return v
}

// ErrBusyAfterRetries wraps a retry-exhausted busy error so callers can
// distinguish "we tried, the lock is real" from a raw SQLITE_BUSY. Not
// currently matched by the appshell error mapper (which still treats it
// as a 500-class error), but gives observability in logs.
var ErrBusyAfterRetries = errors.New("db: busy after retries")