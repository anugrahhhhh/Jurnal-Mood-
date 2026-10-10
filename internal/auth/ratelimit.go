package auth

import (
	"sync"
	"time"
)

// limiter menghitung kegagalan login per kunci dalam jendela waktu tetap.
// Disimpan di memori: cukup untuk satu server; hilang saat restart.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*bucket
}

type bucket struct {
	count int
	reset time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, hits: make(map[string]*bucket)}
}

func (l *limiter) blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.hits[key]
	if !ok || now.After(b.reset) {
		return false
	}
	return b.count >= l.max
}

func (l *limiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.hits[key]
	if !ok || now.After(b.reset) {
		if len(l.hits) > 10_000 { // batasi memori: buang bucket yang sudah lewat
			for k, v := range l.hits {
				if now.After(v.reset) {
					delete(l.hits, k)
				}
			}
		}
		l.hits[key] = &bucket{count: 1, reset: now.Add(l.window)}
		return
	}
	b.count++
}

func (l *limiter) clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
