package bookmark

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// duplicateMode controls how ImportHTML treats a bookmark whose normalized URL
// already exists for the user.
type duplicateMode string

const (
	// dupSkip (default) leaves the existing bookmark untouched and counts the
	// import item as skipped.
	dupSkip duplicateMode = "skip"
	// dupUpdate overwrites the existing bookmark's title/description/category
	// with the imported values.
	dupUpdate duplicateMode = "update"
	// dupDuplicate tries to insert a second row. Because the schema enforces a
	// per-user partial unique index on normalized_url for non-trash rows, items
	// that hit the constraint are reported as skipped rather than crashing.
	dupDuplicate duplicateMode = "duplicate"
)

func parseDuplicateMode(s string) (duplicateMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "skip":
		return dupSkip, nil
	case "update":
		return dupUpdate, nil
	case "duplicate":
		return dupDuplicate, nil
	}
	return "", ValidationError{Field: "duplicateMode", Reason: "must be skip, update, or duplicate"}
}

// maxImportBytes caps the size of an uploaded/imported HTML payload. It is a
// deliberate upper bound to keep parsing bounded; real browser exports are
// well under this. Exposed as a constant so the handler applies the same bound.
const maxImportBytes = 10 * 1024 * 1024 // 10 MiB

// maxImportItems caps the number of bookmarks accepted in a single import, so
// a malicious/huge file cannot exhaust memory or run an unbounded loop.
const maxImportItems = 20000

// ParsedBookmark is a single link extracted from the bookmarks file, with the
// nearest enclosing folder name already resolved.
type ParsedBookmark struct {
	URL      string
	Title    string
	AddDate  string
	Icon     string // favicon data URI or URL, if present in the file
	Category string // nearest enclosing folder name ("" = uncategorized)
}

// parseBookmarksHTML walks a Netscape bookmarks document (Chrome/Edge/Firefox
// export) and returns the flat list of links in document order, each annotated
// with its nearest enclosing folder name.
//
// It uses golang.org/x/net/html's tokenizer rather than a full DOM so the
// parser is forgiving of the loose, non-XHTML markup real browsers emit and
// uses bounded memory.
//
// Nesting model: browsers emit folder content as
//
//	<H3 ...>Folder Name</H3><DL><p> ... </DL>
//
// So the folder header is captured and remembered; when the following <DL>
// opens, the remembered name is pushed onto the folder stack; when that <DL>
// closes, it is popped. A bookmark's category is the innermost folder on the
// stack at the point its <A> closes.
func parseBookmarksHTML(r io.Reader) ([]ParsedBookmark, error) {
	tz := html.NewTokenizer(r)
	tz.SetMaxBuf(maxImportBytes + 1024) // hard tokenizer limit; parse bound

	// folderStack holds the names of currently-open folders, innermost last.
	// "remembered" holds a folder header seen but not yet pushed (waiting for
	// its <DL>). This separates a header that has no children from a real one.
	folderStack := []string{}
	var rememberedFolder string
	rememberedFolderIsH3 := false

	// Anchor state: gathered across the StartTag/text/end-tag sequence.
	var aHref, aIcon, aTitle string
	inAnchor := false

	var out []ParsedBookmark

	for {
		tt := tz.Next()
		if tt == html.ErrorToken {
			if errors.Is(tz.Err(), io.EOF) {
				break
			}
			return nil, fmt.Errorf("import: tokenize html: %w", tz.Err())
		}

		switch tt {
		case html.StartTagToken:
			name, hasAttr := tz.TagName()
			tag := strings.ToLower(string(name))
			switch tag {
			case "dl":
				// Opening a new folder body. Push the remembered header if any;
				// otherwise this is the top-level <DL> with no folder name.
				if rememberedFolder != "" && rememberedFolderIsH3 {
					folderStack = append(folderStack, rememberedFolder)
				}
				rememberedFolder = ""
				rememberedFolderIsH3 = false
			case "h3":
				// A real folder header. Remember it and push on the next <DL>.
				rememberedFolder = ""
				rememberedFolderIsH3 = true
			case "h1":
				// Document/root title (e.g. "Bookmarks bar"). It is NOT a category:
				// treating it as one would put every top-level bookmark under it.
				rememberedFolder = ""
				rememberedFolderIsH3 = false
			case "a":
				if !hasAttr {
					continue
				}
				href, icon := "", ""
				more := true
				for more {
					var k, v []byte
					k, v, more = tz.TagAttr()
					switch strings.ToLower(string(k)) {
					case "href":
						href = string(v)
					case "icon", "icon_uri":
						if icon == "" {
							icon = string(v)
						}
					}
				}
				aHref, aIcon, aTitle = href, icon, ""
				inAnchor = href != ""
			}

		case html.TextToken:
			text := strings.TrimSpace(string(tz.Text()))
			if text == "" {
				continue
			}
			if inAnchor && aHref != "" {
				// Anchor text = bookmark title. Titles can span multiple text
				// tokens (entities split); accumulate.
				if aTitle == "" {
					aTitle = text
				} else {
					aTitle += " " + text
				}
			} else if rememberedFolder == "" && rememberedFolderIsH3 {
				// Tentatively capture folder header text; confirmed when the
				// following <DL> pushes it.
				rememberedFolder = text
			}

		case html.EndTagToken:
			name, _ := tz.TagName()
			switch strings.ToLower(string(name)) {
			case "a":
				if inAnchor && aHref != "" {
					out = append(out, ParsedBookmark{
						URL:      aHref,
						Title:    aTitle,
						Icon:     aIcon,
						Category: nearestFolder(folderStack),
					})
				}
				inAnchor = false
				aHref, aIcon, aTitle = "", "", ""
			case "dl":
				// Closing a folder body: pop the innermost folder (if any). The
				// root <DL> has no corresponding header pushed, so only pop when
				// the stack is non-empty.
				if len(folderStack) > 0 {
					folderStack = folderStack[:len(folderStack)-1]
				}
			}
		}
	}
	return out, nil
}

// nearestFolder returns the innermost folder name on the stack, or "" when the
// bookmark has no enclosing folder.
func nearestFolder(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return stack[len(stack)-1]
}

// ImportResult is the outcome of an import run, returned to the client.
//
// Counting semantics:
//   - Total     = number of bookmarks the parser extracted (the full set).
//   - Processed = number of items actually evaluated (min(Total, itemLimit)).
//     When the input exceeds the per-import item cap, processing stops early
//     and LimitReached is set; the unprocessed overflow is counted as Failed.
//   - Created/Skipped/Updated/Failed partition ALL items, so they always sum
//     to Total (Created+Skipped+Updated+Failed == Total).
type ImportResult struct {
	Total        int           `json:"total"`
	Processed    int           `json:"processed"`
	Created      int           `json:"created"`
	Skipped      int           `json:"skipped"`
	Updated      int           `json:"updated"`
	Failed       int           `json:"failed"`
	LimitReached bool          `json:"limitReached,omitempty"`
	Duplicate    duplicateMode `json:"duplicateMode"`
	// Samples carries a bounded number of representative reasons (skipped /
	// failed) so the response stays small. At most maxResultSamples entries.
	Samples []ImportSample `json:"samples,omitempty"`
}

// ImportSample is one human-readable reason line, used in Samples.
type ImportSample struct {
	Kind   string `json:"kind"` // "skipped" | "failed"
	URL    string `json:"url,omitempty"`
	Reason string `json:"reason"`
}

// maxResultSamples bounds how many reason lines are kept in ImportResult so a
// 20k-item import does not produce a megabyte response.
const maxResultSamples = 50

// ValidateDuplicateMode parses a duplicate-mode string for the handler (returning
// a 400 ValidationError on a bad value) without exposing the unexported type.
func ValidateDuplicateMode(s string) (string, error) {
	m, err := parseDuplicateMode(s)
	if err != nil {
		return "", err
	}
	return string(m), nil
}
