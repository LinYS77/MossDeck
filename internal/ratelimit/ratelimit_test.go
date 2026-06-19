package ratelimit

import (
	"context"
	"testing"
	"time"
)

func newLimiter(t *testing.T, max int, window time.Duration) Limiter {
	t.Helper()
	return New(context.Background(), max, window)
}

func TestAllowsUntilThreshold(t *testing.T) {
	l := newLimiter(t, 3, time.Minute)

	for i := 1; i <= 3; i++ {
		if blocked, _ := l.TooMany("k"); blocked {
			t.Fatalf("attempt %d: unexpectedly blocked", i)
		}
		l.RecordFailure("k")
	}
	// After 3 failures, the key is throttled.
	if blocked, retry := l.TooMany("k"); !blocked {
		t.Fatal("expected to be blocked after threshold failures")
	} else if retry <= 0 {
		t.Fatalf("expected positive retryAfter, got %v", retry)
	}
}

func TestResetClearsLimit(t *testing.T) {
	l := newLimiter(t, 2, time.Minute)
	l.RecordFailure("k")
	l.RecordFailure("k")
	if blocked, _ := l.TooMany("k"); !blocked {
		t.Fatal("expected blocked before reset")
	}
	l.Reset("k")
	if blocked, _ := l.TooMany("k"); blocked {
		t.Fatal("expected not blocked after reset")
	}
}

func TestDistinctKeysAreIndependent(t *testing.T) {
	l := newLimiter(t, 1, time.Minute)
	l.RecordFailure("a")
	if blocked, _ := l.TooMany("a"); !blocked {
		t.Fatal("expected a blocked")
	}
	if blocked, _ := l.TooMany("b"); blocked {
		t.Fatal("expected b not blocked")
	}
}

func TestWindowExpiryUnblocks(t *testing.T) {
	// Use a tiny window so expiry happens within the test without sleeping.
	l := newLimiter(t, 1, 30*time.Millisecond)
	l.RecordFailure("k")
	if blocked, _ := l.TooMany("k"); !blocked {
		t.Fatal("expected blocked within window")
	}
	time.Sleep(45 * time.Millisecond)
	if blocked, _ := l.TooMany("k"); blocked {
		t.Fatal("expected unblocked after window elapsed")
	}
}

func TestRecordFailureStartsNewWindow(t *testing.T) {
	l := newLimiter(t, 2, 30*time.Millisecond)
	l.RecordFailure("k")
	time.Sleep(45 * time.Millisecond)
	// First failure after expiry opens a fresh window; count should be 1.
	l.RecordFailure("k")
	if blocked, _ := l.TooMany("k"); blocked {
		t.Fatal("expected not blocked with count 1 in a fresh window")
	}
}

func TestSweepEvictsExpired(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := New(ctx, 1, 20*time.Millisecond).(*inMemory)
	l.RecordFailure("k")
	time.Sleep(60 * time.Millisecond) // > one sweep tick
	l.sweepOnce()
	l.mu.Lock()
	_, present := l.entries["k"]
	l.mu.Unlock()
	if present {
		t.Fatal("expected expired entry to be swept")
	}
}
