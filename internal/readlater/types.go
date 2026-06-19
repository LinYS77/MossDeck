// Package readlater implements the read-later backend: items CRUD, state
// transitions, soft-delete/restore, open tracking, and tag association.
//
// It mirrors the existing bookmark package layering:
//   - Store: persistence behind an interface.
//   - Service: validation, URL normalization, dedup, state transitions.
//   - handlers: HTTP decoding/encoding and error mapping.
//
// Every operation is scoped by user_id from auth.RequireAuth.
package readlater

import (
	"context"
	"errors"
)

var (
	ErrNotFound     = errors.New("readlater: not found")
	ErrURLTaken     = errors.New("readlater: url already exists")
	ErrInvalidState = errors.New("readlater: invalid state")
	ErrInvalidTagID = errors.New("readlater: tag does not exist")
)

const (
	StateUnread   = "unread"
	StateReading  = "reading"
	StateArchived = "archived"
	StateTrash    = "trash"
)

func isValidState(s string) bool {
	switch s {
	case StateUnread, StateReading, StateArchived, StateTrash:
		return true
	}
	return false
}

type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Reason
	}
	return e.Field + ": " + e.Reason
}

type Tag struct {
	ID        int64
	UserID    int64
	Name      string
	Color     string
	CreatedAt string
	UpdatedAt string
}

type Item struct {
	ID                 int64
	UserID             int64
	URL                string
	NormalizedURL      string
	Title              string
	Excerpt            string
	Author             string
	SiteName           string
	FaviconURL         string
	CoverImageURL      string
	Domain             string
	ReadingTimeMinutes int
	State              string
	Priority           int
	Favorite           bool
	Source             string
	LastOpenedAt       string
	ArchivedAt         string
	MetadataStatus     string
	CreatedAt          string
	UpdatedAt          string
	DeletedAt          string
	Tags               []Tag
}

type CreateParams struct {
	URL                string
	Title              string
	Excerpt            string
	Author             string
	SiteName           string
	FaviconURL         string
	CoverImageURL      string
	ReadingTimeMinutes int
	Priority           int
	Favorite           bool
	Source             string
	TagIDs             []int64
}

type UpdateParams struct {
	URL                *string
	NormalizedURL      *string
	Domain             *string
	Title              *string
	Excerpt            *string
	Author             *string
	SiteName           *string
	FaviconURL         *string
	CoverImageURL      *string
	ReadingTimeMinutes *int
	State              *string
	Priority           *int
	Favorite           *bool
	Source             *string
	TagIDs             *[]int64
}

type ListQuery struct {
	UserID   int64
	Q        string
	State    string
	TagID    int64
	Tag      string
	Domain   string
	Favorite *bool
	Priority *int
	Page     int
	PageSize int
	Sort     string
}

type ListResult struct {
	Items    []Item `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Total    int    `json:"total"`
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type Store interface {
	CreateItem(ctx context.Context, userID int64, item *Item, tagIDs []int64) (*Item, error)
	GetItem(ctx context.Context, userID, id int64) (*Item, error)
	GetItemByNormalizedURL(ctx context.Context, userID int64, normalizedURL string) (*Item, error)
	UpdateItem(ctx context.Context, userID, id int64, p UpdateParams) (*Item, error)
	SetItemState(ctx context.Context, userID, id int64, state string) (*Item, error)
	OpenItem(ctx context.Context, userID, id int64) (*Item, error)
	ListItems(ctx context.Context, q ListQuery) (*ListResult, error)
	DeletePermanently(ctx context.Context, userID, id int64) error
	ListTags(ctx context.Context, userID int64) ([]*Tag, error)
	GetTag(ctx context.Context, userID, id int64) (*Tag, error)
	GetTagByName(ctx context.Context, userID int64, name string) (*Tag, error)
}
