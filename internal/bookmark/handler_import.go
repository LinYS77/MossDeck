package bookmark

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/linyusheng/homepage/internal/api"
)

// handleImportHTML accepts a Netscape bookmarks export via either:
//   - multipart/form-data with a "file" field (browser upload shape), or
//   - application/json { "html": "<...>" } (convenient for tests/curl).
//
// Optional query parameter `duplicateMode` selects skip (default) | update |
// duplicate. The decoded HTML payload is bounded by maxImportBytes; anything
// larger is a 413. The raw HTML is never persisted.
func (s *Service) handleImportHTML() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modeStr := r.URL.Query().Get("duplicateMode")
		mode, err := parseDuplicateMode(modeStr)
		if err != nil {
			api.WriteError(w, r, api.BadRequest(err.Error(), nil))
			return
		}

		reader, closer, err := importReader(r)
		if err != nil {
			// An oversized HTML payload (multipart or JSON) is a 413, with a clear
			// code/message; everything else from importReader is a 400 (bad
			// content-type, malformed JSON, missing file, empty html).
			if errors.Is(err, errImportHTMLTooLarge) {
				api.WriteError(w, r, api.New(http.StatusRequestEntityTooLarge,
					"PAYLOAD_TOO_LARGE",
					"import html exceeds the "+strconv.FormatInt(maxImportBytes/(1<<20), 10)+" MiB limit", err))
				return
			}
			api.WriteError(w, r, api.BadRequest(err.Error(), err))
			return
		}
		defer closer()

		// Bound the reader so a huge upload cannot exhaust memory. (Multipart
		// files are already size-checked; this additionally bounds the stream
		// the tokenizer reads.)
		body := http.MaxBytesReader(w, io.NopCloser(reader), maxImportBytes)

		res, err := s.ImportHTML(r.Context(), currentUserID(r), body, mode)
		if err != nil {
			// A tokenizer buffer-overflow (SetMaxBuf) maps to 413; anything else
			// is internal to avoid leaking details.
			if strings.Contains(err.Error(), "max buffer size") {
				api.WriteError(w, r, api.New(http.StatusRequestEntityTooLarge,
					"PAYLOAD_TOO_LARGE",
					"import body exceeds the "+strconv.FormatInt(maxImportBytes/(1<<20), 10)+" MiB limit", err))
				return
			}
			s.writeInternal(w, r, err, "import html")
			return
		}
		api.WriteOK(w, r, res)
	}
}

// maxJSONBodyBytes bounds the raw JSON request body. It is maxImportBytes
// plus headroom for the JSON envelope (the {"html":"..."} wrapper, key,
// quotes, escaping). The authoritative limit on the HTML payload itself is
// enforced separately below via len(html).
const maxJSONBodyBytes = maxImportBytes + 64*1024

// maxMultipartBodyBytes bounds the raw multipart body before parsing. It allows
// the 10 MiB file plus normal multipart headers/boundaries, while preventing a
// giant body from being parsed or spilled to disk before we inspect the file.
const maxMultipartBodyBytes = maxImportBytes + 64*1024

// errImportHTMLTooLarge is returned by importReader when the decoded JSON
// "html" field exceeds maxImportBytes. The handler maps it to 413.
var errImportHTMLTooLarge = errors.New("html payload exceeds the import size limit")

// importReader extracts the HTML body from either a multipart upload or a JSON
// object, returning a reader and a closer the caller must defer.
func importReader(r *http.Request) (io.Reader, func(), error) {
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "multipart/form-data"):
		r.Body = http.MaxBytesReader(nil, r.Body, maxMultipartBodyBytes)
		if err := r.ParseMultipartForm(maxImportBytes); err != nil {
			if isMaxBytesError(err) {
				return nil, noopCloser, errImportHTMLTooLarge
			}
			return nil, noopCloser, errors.New("could not parse multipart form: " + err.Error())
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return nil, noopCloser, errors.New("missing 'file' field in multipart form")
		}
		// Even though ParseMultipartForm is bounded, double-check the declared
		// size to reject obviously oversized files early.
		if header.Size > maxImportBytes {
			_ = file.Close()
			return nil, noopCloser, errImportHTMLTooLarge
		}
		return file, func() { _ = file.Close() }, nil

	case strings.HasPrefix(ct, "application/json"):
		var body struct {
			HTML string `json:"html"`
		}
		// Allow the envelope + headroom, then enforce the real HTML limit on the
		// decoded value (Plan A): a body that decodes fine but whose html field
		// alone exceeds maxImportBytes is rejected with 413, not accepted.
		r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBodyBytes)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			if isMaxBytesError(err) {
				return nil, noopCloser, errImportHTMLTooLarge
			}
			return nil, noopCloser, errors.New("invalid JSON body: " + err.Error())
		}
		// Authoritative per-spec check: the HTML string itself is bounded.
		if len(body.HTML) > maxImportBytes {
			return nil, noopCloser, errImportHTMLTooLarge
		}
		if strings.TrimSpace(body.HTML) == "" {
			return nil, noopCloser, errors.New("'html' field is required and must not be empty")
		}
		return strings.NewReader(body.HTML), noopCloser, nil

	default:
		return nil, noopCloser, errors.New("Content-Type must be multipart/form-data or application/json")
	}
}

// noopCloser is a no-op closer for readers that do not need releasing.
func noopCloser() {}

func isMaxBytesError(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}
