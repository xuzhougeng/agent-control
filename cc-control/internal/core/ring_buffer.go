package core

import "sync"

type RingBuffer struct {
	mu       sync.RWMutex
	data     []byte
	capacity int
}

func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 64 * 1024
	}
	return &RingBuffer{capacity: capacity}
}

func (r *RingBuffer) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data = append(r.data, p...)
	if len(r.data) > r.capacity {
		r.data = append([]byte(nil), r.data[len(r.data)-r.capacity:]...)
	}
}

func (r *RingBuffer) Snapshot() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.data) == 0 {
		return nil
	}
	return append([]byte(nil), r.data...)
}
