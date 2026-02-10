package core

import (
	"sync"
	"time"
)

type tokenWindow struct {
	start time.Time
	count int
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]tokenWindow
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 200
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]tokenWindow),
	}
}

func (r *RateLimiter) Allow(key string) bool {
	if key == "" {
		key = "anonymous"
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.buckets[key]
	if b.start.IsZero() || now.Sub(b.start) >= r.window {
		r.buckets[key] = tokenWindow{start: now, count: 1}
		return true
	}
	if b.count >= r.limit {
		return false
	}
	b.count++
	r.buckets[key] = b
	return true
}
