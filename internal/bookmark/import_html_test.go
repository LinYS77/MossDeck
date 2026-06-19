package bookmark

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sampleChromeExport is a compact, realistic Netscape export as Chrome emits.
const sampleChromeExport = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks bar</H1>
<DL><p>
    <DT><A HREF="https://go.dev/" ADD_DATE="1700000000" ICON="data:image/png;base64,go">Go</A>
    <DT><A HREF="https://github.com/">GitHub</A>
    <DT><H3 ADD_DATE="1700000000">Dev</H3>
    <DL><p>
        <DT><A HREF="https://pkg.go.dev/" ADD_DATE="1700000000">Go packages</A>
        <DT><H3>Deep</H3>
        <DL><p>
            <DT><A HREF="https://blog.golang.org/">Go Blog</A>
        </DL><p>
    </DL><p>
    <DT><A HREF="javascript:void(0)">should skip</A>
    <DT><A HREF="file:///etc/passwd">should skip too</A>
    <DT><A HREF="https://go.dev/">Duplicate in file</A>
</DL><p>`

// =====================================================================
// Parser unit tests (pure, no DB)
// =====================================================================

func TestParseBookmarksHTMLChromeStyle(t *testing.T) {
	got, err := parseBookmarksHTML(strings.NewReader(sampleChromeExport))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Expected links: go.dev, github.com, pkg.go.dev (Dev), blog.golang.org
	// (Dev/Deep), javascript(kept by parser, filtered at import), file(...),
	// duplicate go.dev. Parser keeps all <A> with an href.
	if len(got) != 7 {
		t.Fatalf("expected 7 parsed anchors, got %d: %+v", len(got), got)
	}
	// The FIRST go.dev anchor has title "Go" (the second, duplicate-in-file,
	// is also returned, in order — dedup happens at import time, not parse).
	if got[0].URL != "https://go.dev/" || got[0].Title != "Go" {
		t.Errorf("first anchor = %+v, want go.dev/\"Go\"", got[0])
	}
	if got[6].URL != "https://go.dev/" || got[6].Title != "Duplicate in file" {
		t.Errorf("last anchor = %+v, want duplicate go.dev", got[6])
	}
	byURL := map[string]ParsedBookmark{} // category checks use first occurrence
	for _, b := range got {
		if _, seen := byURL[b.URL]; !seen {
			byURL[b.URL] = b
		}
	}
	if b := byURL["https://github.com/"]; b.Category != "" {
		t.Errorf("github.com should be uncategorized (H1 root ignored), got category %q", b.Category)
	}
	if b := byURL["https://pkg.go.dev/"]; b.Category != "Dev" {
		t.Errorf("pkg.go.dev category = %q, want Dev", b.Category)
	}
	// Nested: blog.golang.org is under Dev/Deep -> nearest folder "Deep".
	if b := byURL["https://blog.golang.org/"]; b.Category != "Deep" {
		t.Errorf("blog.golang.org category = %q, want Deep (nearest folder)", b.Category)
	}
}

func TestParseBookmarksHTMLFirefoxStyle(t *testing.T) {
	// Firefox uses <H3> with PERSONAL_TOOLBAR_FOLDER and slightly different
	// whitespace, but the same DL/H3/A structure.
	const ff = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
    <DT><H3 PERSONAL_TOOLBAR_FOLDER="true">Bookmarks Menu</H3>
    <DL><p>
        <DT><A HREF="https://example.com/" ICON_URI="https://example.com/favicon.ico">Example</A>
        <DT><H3>Sub</H3>
        <DL><p>
            <DT><A HREF="https://news.ycombinator.com/">HN</A>
        </DL><p>
    </DL><p>
</DL><p>`
	got, err := parseBookmarksHTML(strings.NewReader(ff))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byURL := map[string]ParsedBookmark{}
	for _, b := range got {
		byURL[b.URL] = b
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 anchors, got %d", len(got))
	}
	if b := byURL["https://example.com/"]; b.Title != "Example" || b.Icon == "" {
		t.Errorf("example.com: title=%q icon-len=%d", b.Title, len(b.Icon))
	}
	// example.com is under "Bookmarks Menu" -> nearest is "Bookmarks Menu".
	if b := byURL["https://example.com/"]; b.Category != "Bookmarks Menu" {
		t.Errorf("example.com category = %q, want Bookmarks Menu", b.Category)
	}
	if b := byURL["https://news.ycombinator.com/"]; b.Category != "Sub" {
		t.Errorf("HN category = %q, want Sub", b.Category)
	}
}

func TestParseBookmarksHTMLEmptyAndGarbage(t *testing.T) {
	for _, in := range []string{"", "just text no anchors", "<html><body></body></html>"} {
		got, err := parseBookmarksHTML(strings.NewReader(in))
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 anchors for %q, got %d", in, len(got))
		}
	}
}

// =====================================================================
// Integration tests (real DB + real handlers via the bookmark env)
// =====================================================================

func importJSON(t *testing.T, e *testEnv, html, mode string) (*httptest.ResponseRecorder, ImportResult) {
	t.Helper()
	q := ""
	if mode != "" {
		q = "?duplicateMode=" + mode
	}
	body, _ := json.Marshal(map[string]string{"html": html})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks/import/html"+q, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if e.cookie != nil {
		req.AddCookie(e.cookie)
		if e.csrfCookie != nil {
			req.AddCookie(e.csrfCookie)
			req.Header.Set("X-CSRF-Token", e.csrfCookie.Value)
		}
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	var env envelope
	var res ImportResult
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		_ = json.Unmarshal(env.Data, &res)
	}
	return rec, res
}

func TestImportHTMLHappyPath(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	rec, res := importJSON(t, e, sampleChromeExport, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	// go.dev + github.com at top level; pkg.go.dev (Dev); blog.golang.org (Deep).
	// javascript + file skipped; the second go.dev skipped as in-file dup.
	// So created = 4, skipped = 3.
	if res.Created != 4 {
		t.Errorf("created = %d, want 4; %+v", res.Created, res)
	}
	if res.Skipped != 3 {
		t.Errorf("skipped = %d, want 3; %+v", res.Skipped, res)
	}
	if res.Failed != 0 {
		t.Errorf("failed = %d, want 0; %+v", res.Failed, res)
	}
	if res.Total != 7 {
		t.Errorf("total = %d, want 7", res.Total)
	}
	if res.Duplicate != "skip" {
		t.Errorf("duplicateMode = %q, want skip", res.Duplicate)
	}

	// Categories were created for "Dev" and "Deep" (nearest-folder mapping),
	// but NOT for the empty top level.
	cats := mustData[[]categoryDTO](t, e.get(t, "/api/v1/categories"))
	names := map[string]bool{}
	for _, c := range cats {
		names[c.Name] = true
	}
	if !names["Dev"] || !names["Deep"] {
		t.Errorf("expected Dev and Deep categories, got %v", names)
	}

	// The nested bookmark landed under its nearest folder.
	bm := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?tag="))
	byURL := map[string]bookmarkDTO{}
	for _, b := range bm.Items {
		byURL[b.URL] = b
	}
	if b, ok := byURL["https://blog.golang.org/"]; !ok || b.Title != "Go Blog" {
		t.Errorf("blog.golang.org missing or wrong title: %+v", b)
	}

	// Title fallback to domain works: github.com has a title, but verify the
	// empty-title path by importing an untitled link.
	rec2, res2 := importJSON(t, e, `<DL><p><DT><A HREF="https://no-title.example/"></A></DL><p>`, "")
	if rec2.Code != http.StatusOK {
		t.Fatalf("import2: %d %s", rec2.Code, rec2.Body.String())
	}
	if res2.Created != 1 {
		t.Fatalf("import2 created = %d, want 1", res2.Created)
	}
	bm2 := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?q=no-title"))
	if len(bm2.Items) != 1 || bm2.Items[0].Title != "no-title.example" {
		t.Errorf("title fallback to domain failed: %+v", bm2.Items)
	}
}

func TestImportHTMLDuplicateURLSkip(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// Pre-create a bookmark that the import will try to add again.
	e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://go.dev/"})

	rec, res := importJSON(t, e, `<DL><p><DT><A HREF="https://go.dev/">Go</A></DL><p>`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if res.Created != 0 || res.Skipped != 1 {
		t.Errorf("expected 0 created / 1 skipped, got created=%d skipped=%d; %+v", res.Created, res.Skipped, res)
	}
	// The existing bookmark must not have been overwritten (title stays as-is).
	bm := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?q=go.dev"))
	if len(bm.Items) != 1 {
		t.Fatalf("expected 1 go.dev bookmark, got %d", len(bm.Items))
	}
	if bm.Items[0].Title == "Go" {
		t.Errorf("skip mode must not overwrite the existing title; got %q", bm.Items[0].Title)
	}
}

func TestImportHTMLDuplicateURLUpdate(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// Existing bookmark with a default title (= domain).
	e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://go.dev/"})

	rec, res := importJSON(t, e, `<DL><p><DT><A HREF="https://go.dev/">Imported Title</A></DL><p>`, "update")
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if res.Updated != 1 || res.Created != 0 {
		t.Errorf("expected 1 updated / 0 created, got updated=%d created=%d", res.Updated, res.Created)
	}
	bm := mustData[ListResultDTO](t, e.get(t, "/api/v1/bookmarks?q=go.dev"))
	if len(bm.Items) != 1 || bm.Items[0].Title != "Imported Title" {
		t.Errorf("update mode must overwrite title; got %+v", bm.Items)
	}
}

func TestImportHTMLSkipsNonHTTPURLs(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")
	html := `<DL><p>
<DT><A HREF="javascript:alert(1)">js</A>
<DT><A HREF="file:///x">file</A>
<DT><A HREF="data:text/html,hi">data</A>
<DT><A HREF="ftp://example.com/">ftp</A>
<DT><A HREF="https://ok.example/">ok</A>
</DL><p>`
	rec, res := importJSON(t, e, html, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if res.Created != 1 || res.Skipped != 4 {
		t.Errorf("expected created=1 skipped=4, got %+v", res)
	}
}

func TestImportHTMLRequiresAuth(t *testing.T) {
	e := newTestEnv(t)
	// Deliberately do NOT log in.
	body, _ := json.Marshal(map[string]string{"html": sampleChromeExport})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks/import/html", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestImportHTMLRequiresCSRF(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")
	body, _ := json.Marshal(map[string]string{"html": sampleChromeExport})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks/import/html", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", rec.Code, rec.Body.String())
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "CSRF_INVALID" {
		t.Fatalf("expected CSRF_INVALID, got %+v", env.Error)
	}
}

func TestImportHTMLRejectsOversizedJSONHTML(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// The HTML field alone exceeds maxImportBytes; the raw JSON body fits
	// within the envelope headroom so it decodes, then the len(html) check
	// trips a 413 PAYLOAD_TOO_LARGE (Plan A: the authoritative limit is on the
	// decoded HTML payload, not just the raw bytes).
	big := strings.Repeat("A", maxImportBytes+1024)
	body, _ := json.Marshal(map[string]string{"html": "<DL><p><A HREF=\"https://x.example/\">" + big + "</A></DL><p>"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks/import/html", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie)
	req.Header.Set("X-CSRF-Token", e.csrfCookie.Value)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized html: status = %d, want 413; body %s", rec.Code, rec.Body.String())
	}
	env := decode(t, rec)
	if env.Error == nil || env.Error.Code != "PAYLOAD_TOO_LARGE" {
		t.Fatalf("expected PAYLOAD_TOO_LARGE, got %+v", env.Error)
	}
}

func TestImportHTMLRejectsOversizedJSONBody(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// A JSON body far beyond the envelope limit is rejected during decode
	// (MaxBytesReader) as 413, keeping size-limit failures unambiguous.
	big := strings.Repeat("A", maxJSONBodyBytes+1024)
	body := []byte("{\"html\":\"" + big + "\"}")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks/import/html", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie)
	req.Header.Set("X-CSRF-Token", e.csrfCookie.Value)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized raw body: status = %d, want 413; body %s", rec.Code, rec.Body.String())
	}
}

func TestImportHTMLRejectsOversizedMultipartBody(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "bookmarks.html")
	if err != nil {
		t.Fatalf("multipart create file: %v", err)
	}
	if _, err := fw.Write([]byte(strings.Repeat("A", maxMultipartBodyBytes+1024))); err != nil {
		t.Fatalf("multipart write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks/import/html", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(e.cookie)
	req.AddCookie(e.csrfCookie)
	req.Header.Set("X-CSRF-Token", e.csrfCookie.Value)
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized multipart: status = %d, want 413; body %s", rec.Code, rec.Body.String())
	}
}

func TestImportHTMLRejectsBadDuplicateMode(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")
	rec, _ := importJSON(t, e, sampleChromeExport, "bogus")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad duplicateMode: status = %d, want 400", rec.Code)
	}
}

func TestImportHTMLCategoryReuse(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// Create the category ahead of time.
	e.post(t, "/api/v1/categories", map[string]any{"name": "Dev"})

	// Import bookmarks under a "Dev" folder; the existing category must be
	// reused (not duplicated, not failing on conflict).
	rec, res := importJSON(t, e,
		`<DL><p><DT><H3>Dev</H3><DL><p><DT><A HREF="https://a.example/">a</A></DL><p></DL><p>`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if res.Created != 1 || res.Failed != 0 {
		t.Errorf("expected 1 created / 0 failed, got %+v", res)
	}
	cats := mustData[[]categoryDTO](t, e.get(t, "/api/v1/categories"))
	count := 0
	for _, c := range cats {
		if c.Name == "Dev" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 'Dev' category (reuse), got %d", count)
	}
}

// TestImportHTMLSampleBounded confirms the samples list never grows unbounded
// even with many skipped items.
func TestImportHTMLSampleBounded(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// 200 distinct non-http URLs -> all skipped, but samples capped.
	var sb strings.Builder
	sb.WriteString("<DL><p>")
	for i := 0; i < 200; i++ {
		sb.WriteString(`<DT><A HREF="javascript:x` + itoa(int64(i)) + `">t</A>`)
	}
	sb.WriteString("</DL><p>")
	_, res := importJSON(t, e, sb.String(), "")
	if res.Skipped != 200 {
		t.Errorf("skipped = %d, want 200", res.Skipped)
	}
	if len(res.Samples) > maxResultSamples {
		t.Errorf("samples = %d, must be <= %d", len(res.Samples), maxResultSamples)
	}
}

// Ensure the context import is exercised even if a future edit removes a call.
var _ = context.Background

// TestImportHTMLItemLimitSemantics verifies the counting semantics: Total
// reports the full parsed count honestly, processing stops at the item cap,
// LimitReached is set, the overflow items are counted as failed, and the four
// counters reconcile to Total. Uses a small per-test cap so it stays fast.
func TestImportHTMLItemLimitSemantics(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// Temporarily lower the service's per-import cap for a fast, deterministic
	// truncation test (default maxImportItems is 20k and would create that many
	// rows, making the test slow).
	const cap = 5
	e.svc.itemLimit = cap
	t.Cleanup(func() { e.svc.itemLimit = maxImportItems })

	// 7 distinct valid URLs > cap of 5: 5 created, 2 overflow counted as failed.
	const n = 7
	var sb strings.Builder
	sb.WriteString("<DL><p>")
	for i := 0; i < n; i++ {
		sb.WriteString(`<DT><A HREF="https://x` + itoa(int64(i)) + `.example/">t</A>`)
	}
	sb.WriteString("</DL><p>")

	_, res := importJSON(t, e, sb.String(), "")

	if res.Total != n {
		t.Errorf("Total = %d, want %d (full parsed count, never hidden)", res.Total, n)
	}
	if !res.LimitReached {
		t.Error("expected LimitReached = true when parsed count exceeds the item cap")
	}
	if res.Processed != cap {
		t.Errorf("Processed = %d, want %d (processing stops at the cap)", res.Processed, cap)
	}
	if res.Created != cap {
		t.Errorf("Created = %d, want %d", res.Created, cap)
	}
	if res.Failed != n-cap {
		t.Errorf("Failed = %d, want %d (the overflow items)", res.Failed, n-cap)
	}
	if got := res.Created + res.Skipped + res.Updated + res.Failed; got != res.Total {
		t.Errorf("created+skipped+updated+failed = %d, want Total = %d", got, res.Total)
	}
}

// TestImportHTMLDuplicateModeDuplicateSkips confirms duplicateMode=duplicate
// counts a URL-already-exists item as SKIPPED (consistent with skip mode and
// the README), not failed.
func TestImportHTMLDuplicateModeDuplicateSkips(t *testing.T) {
	e := newTestEnv(t)
	e.loginOwner(t, "StrongPass1!")

	// Pre-create a bookmark the import will collide with.
	e.post(t, "/api/v1/bookmarks", map[string]any{"url": "https://go.dev/"})

	rec, res := importJSON(t, e, `<DL><p><DT><A HREF="https://go.dev/">Go</A></DL><p>`, "duplicate")
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if res.Created != 0 {
		t.Errorf("created = %d, want 0 (unique index forbids the second row)", res.Created)
	}
	if res.Skipped != 1 || res.Failed != 0 {
		t.Errorf("duplicate mode must count collision as skipped, not failed: skipped=%d failed=%d", res.Skipped, res.Failed)
	}
}
