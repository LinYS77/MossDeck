// Package ratelimit provides login-attempt rate limiting.
//
// It exposes a small Limiter interface so the in-memory implementation used
// today can later be swapped for a persistent (DB) or distributed (Redis)
// backend without touching the auth handlers. The current implementation
// counts failed attempts per key over a fixed window and throttles once a
// threshold is exceeded.
//
// A key is application-defined; the auth layer composes it from the client IP
// and username so that an attacker cannot bypass the limit by rotating one or
// the other alone.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter rate-limits login attempts by opaque key.
//
// The contract is failure-oriented: failures are recorded and counted; a
// successful login resets the key. This mirrors how login abuse detection is
// typically modelled and avoids penalising successful traffic.
type Limiter interface {
	// TooMany reports whether key has exceeded its failure threshold within
	// the window and, if so, how long until the count resets. It does not
	// mutate state.
	TooMany(key string) (blocked bool, retryAfter time.Duration)
	// RecordFailure registers one failed attempt for key, starting a new
	// window if the previous one has elapsed.
	RecordFailure(key string)
	// Reset forgets all recorded failures for key (call on success).
	Reset(key string)
}

// entry holds the failure count and the window start for a single key.
type entry struct {
	count   int
	started time.Time
}

// inMemory is a process-local Limiter. It is safe for concurrent use.
type inMemory struct {
	mu      sync.Mutex
	entries map[string]*entry
	max     int
	window  time.Duration
}

// New returns an in-memory Limiter and starts a background goroutine that
// evicts expired entries to bound memory under sustained abuse. The goroutine
// stops when ctx is cancelled, so callers should pass the application
// lifetime context.
func New(ctx context.Context, max int, window time.Duration) Limiter {
	l := &inMemory{
		entries: make(map[string]*entry),
		max:     max,
		window:  window,
	}
	go l.sweep(ctx)
	return l
}

func (l *inMemory) TooMany(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok {
		return false, 0
	}
	now := time.Now()
	if now.Sub(e.started) >= l.window {
		// Window elapsed: the limit has effectively reset, even though the
		// stale entry is still in the map (the sweeper or the next failure
		// will clean it up).
		return false, 0
	}
	if e.count < l.max {
		return false, 0
	}
	return true, l.window - now.Sub(e.started)
}

func (l *inMemory) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.entries[key]
	if !ok || now.Sub(e.started) >= l.window {
		// New window: (re)create the entry with a fresh start time.
		l.entries[key] = &entry{count: 1, started: now}
		return
	}
	e.count++
}

func (l *inMemory) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// sweep periodically drops expired entries. Running at window granularity is
// sufficient: an expired entry is harmless once its window has elapsed
// (TooMany/RecordFailure both treat it as fresh).
func (l *inMemory) sweep(ctx context.Context) {
	t := time.NewTicker(l.window)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.sweepOnce()
		}
	}
}

func (l *inMemory) sweepOnce() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-l.window)
	for k, e := range l.entries {
		if e.started.Before(cutoff) {
			delete(l.entries, k)
		}
	}
}
