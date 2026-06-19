package bookmark

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// ImportHTML parses a Netscape bookmarks document and imports the links,
// reusing the same URL-normalization, dedup, and category-ownership rules as
// the rest of the bookmark service so an import can never bypass them.
//
// Resilience: a single bad item never aborts the whole run. Items are counted
// as created / skipped / updated / failed, and up to maxResultSamples reason
// lines are retained. The whole run is not wrapped in one transaction because
// a 20k-item import should commit progress incrementally; instead each item
// commits independently (the store methods are transactional per call).
func (s *Service) ImportHTML(ctx context.Context, userID int64, r io.Reader, mode duplicateMode) (*ImportResult, error) {
	parsed, err := parseBookmarksHTML(r)
	if err != nil {
		return nil, err
	}

	// Pre-load the user's categories so we can reuse names without an N+1 of
	// "create -> conflict -> get" round trips. categoryByName is rebuilt as we
	// create new categories during the import.
	cats, err := s.store.ListCategories(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("import: list categories: %w", err)
	}
	categoryByName := make(map[string]int64, len(cats))
	for _, c := range cats {
		if !c.Archived {
			categoryByName[normalizeCategoryName(c.Name)] = c.ID
		}
	}

	res := &ImportResult{Total: len(parsed), Duplicate: mode}
	// Track which normalized URLs we already inserted IN THIS run, so a file
	// containing the same link twice (under two folders) does not create a
	// duplicate row and trip the unique index mid-import.
	seenThisRun := make(map[string]struct{}, len(parsed))

	for i, pb := range parsed {
		// Honest accounting: Total stays the full parsed count. If the input
		// exceeds the item cap, stop processing, flag truncation, and count the
		// overflow items as failed. Reconciliation:
		//   Created+Skipped+Updated+Failed == Total
		//   Processed == min(Total, itemLimit)
		if i >= s.itemLimit {
			res.LimitReached = true
			res.Failed += len(parsed) - i
			res.addSample("failed", pb.URL, fmt.Sprintf(
				"import item limit (%d) reached; %d item(s) not processed", s.itemLimit, len(parsed)-i))
			break
		}
		res.Processed++

		// --- URL validation: only http/https survives; others are skipped. ---
		normalized, domain, ok := NormalizeURL(pb.URL)
		if !ok {
			res.addSample("skipped", pb.URL, "non-http(s) or invalid URL")
			res.Skipped++
			continue
		}

		// --- Dedup within this run (a link may appear twice in the file). ---
		if _, dup := seenThisRun[normalized]; dup {
			res.addSample("skipped", pb.URL, "duplicate within import")
			res.Skipped++
			continue
		}

		// --- Resolve / create the category (reuse by name). ---
		categoryID, ferr := s.resolveImportCategory(ctx, userID, pb.Category, categoryByName)
		if ferr != nil {
			// Category creation is best-effort; a failure downgrades the item to
			// uncategorized rather than failing the bookmark.
			categoryID = 0
		}

		// --- Title default: empty -> domain (matches single-bookmark create). ---
		title := strings.TrimSpace(pb.Title)
		if title == "" {
			title = domain
		}

		// --- Apply the duplicate policy against existing rows. ---
		existing, existErr := s.store.GetBookmarkByNormalizedURL(ctx, userID, normalized)
		if existErr != nil && !isNotFound(existErr) {
			res.addSample("failed", pb.URL, "lookup error")
			res.Failed++
			continue
		}
		exists := !isNotFound(existErr)

		switch {
		case exists && mode == dupSkip:
			res.addSample("skipped", pb.URL, "URL already exists")
			res.Skipped++
			seenThisRun[normalized] = struct{}{}
			continue
		case exists && mode == dupUpdate:
			_, err := s.store.UpdateBookmark(ctx, userID, existing.ID, UpdateBookmarkParams{
				Title:      &title,
				CategoryID: &categoryID,
			})
			if err != nil {
				res.addSample("failed", pb.URL, "update failed")
				res.Failed++
				continue
			}
			res.Updated++
			seenThisRun[normalized] = struct{}{}
			continue
		case exists && mode == dupDuplicate:
			// duplicateMode=duplicate asks for a second row, but the schema's
			// per-user partial unique index forbids two non-trash rows with the
			// same normalized_url. We treat that as a SKIP (the URL is already
			// represented by the existing bookmark), not a failure — consistent
			// with skip mode and the README. (Documented as a no-op mode.)
			_, err := s.store.CreateBookmark(ctx, userID, &Bookmark{
				URL: pb.URL, NormalizedURL: normalized, Title: title,
				Domain: domain, CategoryID: categoryID, Status: StatusActive,
			}, nil)
			if err != nil {
				if isErrBookmarkURLTaken(err) {
					res.addSample("skipped", pb.URL, "URL already exists (duplicate mode)")
					res.Skipped++
				} else {
					res.addSample("failed", pb.URL, "create failed")
					res.Failed++
				}
				continue
			}
			res.Created++
			seenThisRun[normalized] = struct{}{}
			continue
		}

		// --- Default path: create a new bookmark. ---
		_, err = s.store.CreateBookmark(ctx, userID, &Bookmark{
			URL: pb.URL, NormalizedURL: normalized, Title: title,
			Domain: domain, CategoryID: categoryID, Status: StatusActive,
		}, nil)
		if err != nil {
			// A racing insert between our lookup and create would surface as a
			// unique violation; treat that as a skip, not a failure.
			if isErrBookmarkURLTaken(err) {
				res.addSample("skipped", pb.URL, "URL already exists")
				res.Skipped++
				continue
			}
			res.addSample("failed", pb.URL, "create failed")
			res.Failed++
			continue
		}
		res.Created++
		seenThisRun[normalized] = struct{}{}
	}

	return res, nil
}

// resolveImportCategory returns the id of an existing (non-archived) category
// with the given display name, or creates one. categoryByName is an in/out
// cache so repeated links under the same folder do not re-query. An empty name
// yields 0 (uncategorized). Creation conflicts (two threads / re-runs) are
// handled by treating ErrCategoryNameTaken as "reuse the existing one".
func (s *Service) resolveImportCategory(ctx context.Context, userID int64, name string, categoryByName map[string]int64) (int64, error) {
	name = normalizeCategoryName(name)
	if name == "" {
		return 0, nil
	}
	if id, ok := categoryByName[name]; ok {
		return id, nil
	}
	cat, err := s.store.CreateCategory(ctx, userID, CreateCategoryParams{
		Name: name, Type: "bookmark",
	})
	if err != nil {
		if isErrCategoryNameTaken(err) {
			// Reuse the existing category: look it up by name via the list
			// (cheaper than a dedicated method and avoids a new store API).
			if c, lerr := s.findCategoryByName(ctx, userID, name); lerr == nil {
				categoryByName[name] = c
				return c, nil
			}
		}
		return 0, err
	}
	categoryByName[name] = cat.ID
	return cat.ID, nil
}

// findCategoryByName scans the user's categories for one whose normalized name
// matches. Used as the recovery path when CreateCategory races with an existing
// name (so the import never fails on category conflicts).
func (s *Service) findCategoryByName(ctx context.Context, userID int64, name string) (int64, error) {
	cats, err := s.store.ListCategories(ctx, userID)
	if err != nil {
		return 0, err
	}
	for _, c := range cats {
		if normalizeCategoryName(c.Name) == name {
			return c.ID, nil
		}
	}
	return 0, ErrNotFound
}

// addSample appends a reason line to the result, capped at maxResultSamples so
// the JSON response stays bounded.
func (r *ImportResult) addSample(kind, url, reason string) {
	if len(r.Samples) >= maxResultSamples {
		return
	}
	r.Samples = append(r.Samples, ImportSample{Kind: kind, URL: truncateForSample(url), Reason: reason})
}

// truncateForSample clips very long URLs in sample lines to keep them readable.
func truncateForSample(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// normalizeCategoryName trims and collapses whitespace so "Dev  Tools" and
// "Dev Tools" reuse the same category.
func normalizeCategoryName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Collapse runs of internal whitespace.
	var b strings.Builder
	b.Grow(len(name))
	prevSpace := false
	for _, r := range name {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return b.String()
}

// isNotFound reports whether err is the domain ErrNotFound sentinel.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrNotFound
}

// isErrBookmarkURLTaken reports whether err is the URL-taken sentinel.
func isErrBookmarkURLTaken(err error) bool {
	return err == ErrBookmarkURLTaken
}

// isErrCategoryNameTaken reports whether err is the category-name-taken sentinel.
func isErrCategoryNameTaken(err error) bool {
	return err == ErrCategoryNameTaken
}
