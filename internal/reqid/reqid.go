// Package reqid provides per-request identifier propagation via context.
//
// It is intentionally dependency-free so that the api, middleware, and
// handler packages can read the request id without importing one another.
package reqid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey struct{}

// New returns a fresh request id of the form "req_<12 hex chars>".
// It uses crypto/rand so that ids are unguessable, even though they are
// not treated as a secret.
func New() string {
	var b [6]byte
	// Reading from crypto/rand essentially never fails on Linux; if it did,
	// falling back to a zero id is harmless because callers always combine
	// it with other log fields.
	_, _ = rand.Read(b[:])
	return "req_" + hex.EncodeToString(b[:])
}

// With returns a copy of ctx that carries id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the request id stored in ctx, or "" when none is present.
func From(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
