package readlater

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/linyusheng/homepage/internal/auth"
	"github.com/linyusheng/homepage/internal/bookmark"
)

// Service implements read-later business rules. It holds auth only so Register
// can wrap every handler in auth.RequireAuth.
type Service struct {
	store  Store
	auth   *auth.Service
	logger *slog.Logger
}

func NewService(store Store, authSvc *auth.Service, logger *slog.Logger) *Service {
	return &Service{store: store, auth: authSvc, logger: logger}
}

func (s *Service) Create(ctx context.Context, userID int64, p CreateParams) (*Item, error) {
	normalized, domain, ok := validateURL(p.URL)
	if !ok {
		return nil, ValidationError{Field: "url", Reason: "must be a valid http(s) URL"}
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = domain
	}
	if p.ReadingTimeMinutes < 0 {
		return nil, ValidationError{Field: "readingTimeMinutes", Reason: "must be >= 0"}
	}
	tagIDs, err := s.validateOwnedTagIDs(ctx, userID, p.TagIDs)
	if err != nil {
		return nil, err
	}
	item := &Item{
		URL:                strings.TrimSpace(p.URL),
		NormalizedURL:      normalized,
		Title:              title,
		Excerpt:            strings.TrimSpace(p.Excerpt),
		Author:             strings.TrimSpace(p.Author),
		SiteName:           strings.TrimSpace(p.SiteName),
		FaviconURL:         strings.TrimSpace(p.FaviconURL),
		CoverImageURL:      strings.TrimSpace(p.CoverImageURL),
		Domain:             domain,
		ReadingTimeMinutes: p.ReadingTimeMinutes,
		State:              StateUnread,
		Priority:           p.Priority,
		Favorite:           p.Favorite,
		Source:             strings.TrimSpace(p.Source),
	}
	created, err := s.store.CreateItem(ctx, userID, item, tagIDs)
	if err != nil {
		if errors.Is(err, ErrURLTaken) {
			return nil, ErrURLTaken
		}
		return nil, err
	}
	return created, nil
}

func (s *Service) Update(ctx context.Context, userID, id int64, p UpdateParams) (*Item, error) {
	if p.URL != nil {
		normalized, domain, ok := validateURL(*p.URL)
		if !ok {
			return nil, ValidationError{Field: "url", Reason: "must be a valid http(s) URL"}
		}
		existing, err := s.store.GetItem(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if normalized != existing.NormalizedURL {
			other, err := s.store.GetItemByNormalizedURL(ctx, userID, normalized)
			if err == nil && other.ID != id {
				return nil, ErrURLTaken
			}
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		}
		u := strings.TrimSpace(*p.URL)
		p.URL = &u
		p.NormalizedURL = &normalized
		p.Domain = &domain
	}
	if p.Title != nil {
		t := strings.TrimSpace(*p.Title)
		p.Title = &t
	}
	if p.Excerpt != nil {
		v := strings.TrimSpace(*p.Excerpt)
		p.Excerpt = &v
	}
	if p.Author != nil {
		v := strings.TrimSpace(*p.Author)
		p.Author = &v
	}
	if p.SiteName != nil {
		v := strings.TrimSpace(*p.SiteName)
		p.SiteName = &v
	}
	if p.FaviconURL != nil {
		v := strings.TrimSpace(*p.FaviconURL)
		p.FaviconURL = &v
	}
	if p.CoverImageURL != nil {
		v := strings.TrimSpace(*p.CoverImageURL)
		p.CoverImageURL = &v
	}
	if p.Source != nil {
		v := strings.TrimSpace(*p.Source)
		p.Source = &v
	}
	if p.ReadingTimeMinutes != nil && *p.ReadingTimeMinutes < 0 {
		return nil, ValidationError{Field: "readingTimeMinutes", Reason: "must be >= 0"}
	}
	if p.State != nil && !isValidState(*p.State) {
		return nil, ErrInvalidState
	}
	if p.TagIDs != nil {
		owned, err := s.validateOwnedTagIDs(ctx, userID, *p.TagIDs)
		if err != nil {
			return nil, err
		}
		p.TagIDs = &owned
	}
	return s.store.UpdateItem(ctx, userID, id, p)
}

func (s *Service) Trash(ctx context.Context, userID, id int64) (*Item, error) {
	return s.store.SetItemState(ctx, userID, id, StateTrash)
}

func (s *Service) Restore(ctx context.Context, userID, id int64) (*Item, error) {
	return s.store.SetItemState(ctx, userID, id, StateUnread)
}

func (s *Service) Archive(ctx context.Context, userID, id int64) (*Item, error) {
	return s.store.SetItemState(ctx, userID, id, StateArchived)
}

// Purge hard-deletes a trashed item. It fails when the item is not in trash
// state or does not belong to the user.
func (s *Service) Purge(ctx context.Context, userID, id int64) error {
	return s.store.DeletePermanently(ctx, userID, id)
}

// Open records last_opened_at and moves unread items to reading.
func (s *Service) Open(ctx context.Context, userID, id int64) (*Item, error) {
	return s.store.OpenItem(ctx, userID, id)
}

func (s *Service) List(ctx context.Context, q ListQuery) (*ListResult, error) {
	if q.State != "" && !isValidState(q.State) {
		return nil, ErrInvalidState
	}
	if q.Tag != "" && q.TagID == 0 {
		tag, err := s.store.GetTagByName(ctx, q.UserID, q.Tag)
		if err == nil {
			q.TagID = tag.ID
		} else if errors.Is(err, ErrNotFound) {
			return &ListResult{Items: []Item{}, Page: max1(q.Page), PageSize: pageSizeOrDefault(q.PageSize), Total: 0}, nil
		} else {
			return nil, err
		}
	}
	return s.store.ListItems(ctx, q)
}

func (s *Service) validateOwnedTagIDs(ctx context.Context, userID int64, tagIDs []int64) ([]int64, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	out := make([]int64, 0, len(tagIDs))
	seen := make(map[int64]struct{}, len(tagIDs))
	for _, id := range tagIDs {
		if id <= 0 {
			return nil, ValidationError{Field: "tagIds", Reason: "must contain positive ids"}
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, err := s.store.GetTag(ctx, userID, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, ValidationError{Field: "tagIds", Reason: "tag does not exist"}
			}
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func validateURL(raw string) (normalized, domain string, ok bool) {
	return bookmark.NormalizeURL(raw)
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

func pageSizeOrDefault(v int) int {
	if v <= 0 {
		return defaultPageSize
	}
	if v > maxPageSize {
		return maxPageSize
	}
	return v
}
