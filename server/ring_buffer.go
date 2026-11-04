package server

import "sync"

const DefaultRingBufferSize = 1024 * 1024 // 1MB default circular buffer capacity

// CircularBuffer is a thread-safe fixed-capacity byte ring buffer.
type CircularBuffer struct {
	mu       sync.RWMutex
	buf      []byte
	capacity int
	start    int
	length   int
}

// NewCircularBuffer creates a new CircularBuffer with the given capacity in bytes.
func NewCircularBuffer(capacity int) *CircularBuffer {
	if capacity <= 0 {
		capacity = DefaultRingBufferSize
	}
	return &CircularBuffer{
		buf:      make([]byte, capacity),
		capacity: capacity,
	}
}

// Write appends p into the circular buffer, overwriting the oldest bytes if capacity is exceeded.
func (cb *CircularBuffer) Write(p []byte) (int, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	n := len(p)
	if n == 0 {
		return 0, nil
	}

	// If p is larger than or equal to capacity, only keep the last capacity bytes.
	if n >= cb.capacity {
		copy(cb.buf, p[n-cb.capacity:])
		cb.start = 0
		cb.length = cb.capacity
		return n, nil
	}

	// Write bytes into the ring
	writePos := (cb.start + cb.length) % cb.capacity
	firstCopy := cb.capacity - writePos
	if n <= firstCopy {
		copy(cb.buf[writePos:writePos+n], p)
	} else {
		copy(cb.buf[writePos:], p[:firstCopy])
		copy(cb.buf[:n-firstCopy], p[firstCopy:])
	}

	if cb.length+n <= cb.capacity {
		cb.length += n
	} else {
		overflow := (cb.length + n) - cb.capacity
		cb.start = (cb.start + overflow) % cb.capacity
		cb.length = cb.capacity
	}

	return n, nil
}

// Bytes returns a chronological copy of all buffered bytes.
func (cb *CircularBuffer) Bytes() []byte {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.length == 0 {
		return nil
	}

	out := make([]byte, cb.length)
	firstPart := cb.capacity - cb.start
	if cb.length <= firstPart {
		copy(out, cb.buf[cb.start:cb.start+cb.length])
	} else {
		copy(out[:firstPart], cb.buf[cb.start:])
		copy(out[firstPart:], cb.buf[:cb.length-firstPart])
	}
	return out
}

// Len returns the current number of stored bytes.
func (cb *CircularBuffer) Len() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.length
}

// Reset clears the buffer.
func (cb *CircularBuffer) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.start = 0
	cb.length = 0
}
