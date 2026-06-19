package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newReq() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	return r
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return env
}

func TestWriteOK(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteOK(rec, newReq(), map[string]string{"status": "ok"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	env := decode(t, rec)
	if env.Error != nil {
		t.Fatalf("expected no error, got %+v", env.Error)
	}
	if env.Data == nil {
		t.Fatal("expected data, got nil")
	}
	// No request id was set on the context, so requestId should be "".
	if env.RequestID != "" {
		t.Fatalf("expected empty requestId, got %q", env.RequestID)
	}
}

func TestWriteErrorTyped(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, newReq(), NotFound("bookmark not found"))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "NOT_FOUND" || env.Error.Message != "bookmark not found" {
		t.Fatalf("unexpected error envelope: %+v", env.Error)
	}
	if env.Data != nil {
		t.Fatalf("expected nil data on error, got %v", env.Data)
	}
}

func TestWriteErrorOpaqueBecomesInternal(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, newReq(), errBoom{})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "INTERNAL" {
		t.Fatalf("expected INTERNAL, got %+v", env.Error)
	}
	if env.Error.Message != "internal server error" {
		t.Fatalf("unexpected message leaking details: %q", env.Error.Message)
	}
}

func TestStatusOf(t *testing.T) {
	if got := StatusOf(BadRequest("x", nil)); got != http.StatusBadRequest {
		t.Fatalf("got %d", got)
	}
	if got := StatusOf(errBoom{}); got != http.StatusInternalServerError {
		t.Fatalf("got %d", got)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
