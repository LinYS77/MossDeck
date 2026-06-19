package reqid

import (
	"context"
	"testing"
)

func TestNewFormat(t *testing.T) {
	id := New()
	if len(id) != len("req_")+12 {
		t.Fatalf("unexpected id length: %q (%d)", id, len(id))
	}
	if id[:4] != "req_" {
		t.Fatalf("id %q missing req_ prefix", id)
	}
}

func TestNewIsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := New()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestFromRoundTrip(t *testing.T) {
	if got := From(context.Background()); got != "" {
		t.Fatalf("expected empty id from bare context, got %q", got)
	}
	ctx := With(context.Background(), "req_deadbeef")
	if got := From(ctx); got != "req_deadbeef" {
		t.Fatalf("expected round-trip id, got %q", got)
	}
}
