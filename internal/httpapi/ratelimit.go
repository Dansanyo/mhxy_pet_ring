package httpapi

import (
	"sync"
	"time"
)

type limitEntry struct {
	windowStart time.Time
	count       int
}

type fixedWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	items  map[string]limitEntry
}

func newFixedWindowLimiter(limit int, window time.Duration, now func() time.Time) *fixedWindowLimiter {
	return &fixedWindowLimiter{limit: limit, window: window, now: now, items: make(map[string]limitEntry)}
}

func (l *fixedWindowLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.now()
	entry := l.items[key]
	if entry.windowStart.IsZero() || current.Sub(entry.windowStart) >= l.window {
		l.items[key] = limitEntry{windowStart: current, count: 1}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.items[key] = entry
	return true
}
