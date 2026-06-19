// Package api defines the HTTP wire contract shared by every endpoint:
// the uniform response envelope, the structured error model, and helpers
// for writing JSON responses. Centralizing the format here lets future
// feature modules (auth, bookmark, import) emit consistent responses and
// keeps handlers thin.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/linyusheng/homepage/internal/reqid"
)

// Envelope is the JSON object returned by every API endpoint.
// Exactly one of Data or Error is set for any given response.
type Envelope struct {
	Data      any    `json:"data,omitempty"`
	Error     *Error `json:"error,omitempty"`
	RequestID string `json:"requestId"`
}

// Error is the structured error body inside an Envelope.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details carries optional, endpoint-specific structured detail and is
	// omitted when nil.
	Details any `json:"details,omitempty"`
}

// StatusError is an application error that maps to an HTTP status and a
// stable machine-readable code. Handlers build one via the constructors
// below (e.g. BadRequest, NotFound) and pass it to WriteError.
type StatusError struct {
	Status int    `json:"-"`
	Code   string `json:"-"`
	Msg    string `json:"-"`
	// Cause is the underlying error, if any. It is logged but never
	// serialized to clients.
	Cause error `json:"-"`
}

func (e *StatusError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

func (e *StatusError) Unwrap() error { return e.Cause }

// Common constructors for the error categories used across the API.
// Additional, feature-specific codes (e.g. AUTH_*) can be added later.

// New constructs a StatusError directly. Prefer the named constructors.
func New(status int, code, msg string, cause error) *StatusError {
	return &StatusError{Status: status, Code: code, Msg: msg, Cause: cause}
}

// BadRequest is a 400 for malformed or invalid input.
func BadRequest(msg string, cause error) *StatusError {
	return &StatusError{Status: http.StatusBadRequest, Code: "BAD_REQUEST", Msg: msg, Cause: cause}
}

// Unauthorized is a 401 for missing or invalid credentials.
func Unauthorized(msg string) *StatusError {
	return &StatusError{Status: http.StatusUnauthorized, Code: "UNAUTHORIZED", Msg: msg}
}

// Forbidden is a 403 for an authenticated but disallowed action.
func Forbidden(msg string) *StatusError {
	return &StatusError{Status: http.StatusForbidden, Code: "FORBIDDEN", Msg: msg}
}

// NotFound is a 404 for a missing resource.
func NotFound(msg string) *StatusError {
	return &StatusError{Status: http.StatusNotFound, Code: "NOT_FOUND", Msg: msg}
}

// Conflict is a 409 for a duplicate or conflicting resource.
func Conflict(msg string) *StatusError {
	return &StatusError{Status: http.StatusConflict, Code: "CONFLICT", Msg: msg}
}

// TooManyRequests is a 429 returned when a client has exceeded a rate limit.
func TooManyRequests(msg string) *StatusError {
	return &StatusError{Status: http.StatusTooManyRequests, Code: "TOO_MANY_REQUESTS", Msg: msg}
}

// Internal is a 500 for unexpected server-side failures.
func Internal(msg string, cause error) *StatusError {
	return &StatusError{Status: http.StatusInternalServerError, Code: "INTERNAL", Msg: msg, Cause: cause}
}

// WriteOK writes a 200 response with data wrapped in the standard envelope.
func WriteOK(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, http.StatusOK, Envelope{Data: data, RequestID: reqid.From(r.Context())})
}

// WriteCreated writes a 201 response with data, typically the newly created
// resource.
func WriteCreated(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, http.StatusCreated, Envelope{Data: data, RequestID: reqid.From(r.Context())})
}

// WriteError writes err as an error envelope. Errors that are not a
// *StatusError are treated as opaque 500s so that implementation details
// never leak to clients.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var se *StatusError
	if !errors.As(err, &se) {
		se = &StatusError{
			Status: http.StatusInternalServerError,
			Code:   "INTERNAL",
			Msg:    "internal server error",
			Cause:  err,
		}
	}
	writeJSON(w, se.Status, Envelope{
		Error:     &Error{Code: se.Code, Message: se.Msg},
		RequestID: reqid.From(r.Context()),
	})
}

// StatusOf extracts the HTTP status from err if it is a *StatusError, else 500.
// Useful in tests.
func StatusOf(err error) int {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	buf, err := json.Marshal(body)
	if err != nil {
		// Marshalling our own envelope should never fail; fall back to a
		// minimal 500 so the client still gets a valid JSON error.
		http.Error(w, `{"error":{"code":"INTERNAL","message":"encode error"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
	_, _ = w.Write([]byte("\n"))
}
