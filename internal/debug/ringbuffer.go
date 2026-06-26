package debug

import (
	"sync"
	"time"
)

// Entry is a single captured log record. Snapshot, not live reference.
type Entry struct {
	Time      time.Time      `json:"time"`
	Level     string         `json:"level"`
	Message   string         `json:"msg"`
	Component string         `json:"component,omitempty"`
	Source    string         `json:"source,omitempty"` // e.g., "frontend", "internal-test"
	Attrs     map[string]any `json:"attrs,omitempty"`
}

// RingBuffer is a fixed-capacity FIFO. Oldest entry evicted when full.
// Safe for concurrent use.
type RingBuffer struct {
	mu    sync.Mutex
	buf   []Entry
	head  int
	size  int
	cap   int
	total uint64
}

func NewRingBuffer(capacity int) *RingBuffer {
	if capacity < 0 {
		capacity = 0
	}
	return &RingBuffer{
		buf: make([]Entry, capacity),
		cap: capacity,
	}
}

func (r *RingBuffer) Push(e Entry) {
	if r == nil || r.cap == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head] = e
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
	r.total++
}

func (r *RingBuffer) Snapshot() []Entry {
	if r == nil || r.size == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Entry, 0, r.size)
	if r.size < r.cap {
		out = append(out, r.buf[:r.head]...)
	} else {
		out = append(out, r.buf[r.head:]...)
		out = append(out, r.buf[:r.head]...)
	}
	return out
}

// Since returns entries added after the given monotonic counter value.
// Holds the lock across Snapshot + total read so a concurrent Push
// cannot produce a wrong skip calculation.
func (r *RingBuffer) Since(counter uint64) []Entry {
	if r == nil || r.cap == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == 0 || counter >= r.total {
		return nil
	}
	// Build snapshot under the same lock (chronological order).
	all := make([]Entry, 0, r.size)
	if r.size < r.cap {
		all = append(all, r.buf[:r.head]...)
	} else {
		all = append(all, r.buf[r.head:]...)
		all = append(all, r.buf[:r.head]...)
	}
	// counter represents "I've seen up to counter entries total".
	// After (r.total - counter) new pushes, we want only the LAST
	// (r.total - counter) entries from `all`. So skip the first
	// len(all) - (r.total - counter) entries.
	skip := len(all) - int(r.total-counter)
	if skip < 0 {
		skip = 0
	}
	if skip >= len(all) {
		return nil
	}
	return all[skip:]
}

func (r *RingBuffer) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.size
}

func (r *RingBuffer) Cap() int {
	if r == nil {
		return 0
	}
	return r.cap
}

func (r *RingBuffer) Total() uint64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.total
}

func (r *RingBuffer) Clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.buf {
		r.buf[i] = Entry{}
	}
	r.head = 0
	r.size = 0
}