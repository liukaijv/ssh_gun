package applog

import "sync"

// RingBuffer stores a fixed number of the most recently appended log lines.
type RingBuffer struct {
	mu       sync.RWMutex
	lines    []string
	capacity int
	next     int
	full     bool
}

// NewRingBuffer creates a thread-safe buffer. A non-positive capacity stores
// no lines.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity < 0 {
		capacity = 0
	}
	return &RingBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

// Append adds a line, evicting the oldest line when the buffer is full.
func (r *RingBuffer) Append(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capacity == 0 {
		return
	}
	r.lines[r.next] = line
	r.next = (r.next + 1) % r.capacity
	if r.next == 0 {
		r.full = true
	}
}

// Snapshot returns the stored lines in oldest-to-newest order.
func (r *RingBuffer) Snapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.full {
		out := make([]string, r.next)
		copy(out, r.lines[:r.next])
		return out
	}
	out := make([]string, r.capacity)
	copied := copy(out, r.lines[r.next:])
	copy(out[copied:], r.lines[:r.next])
	return out
}
