package bookmark

import (
	"net/url"
	"strings"
)

// NormalizeURL canonicalizes a URL for deduplication and domain extraction.
//
// Rules (deliberately conservative, no network access):
//   - Reject schemes other than http/https.
//   - Lowercase scheme and host.
//   - Strip a trailing slash from the path when the path is "/" (collapse
//     "example.com/" and "example.com"), but keep meaningful trailing
//     slashes elsewhere.
//   - Drop the default port (80/http, 443/https).
//   - Sort query parameters so that parameter order does not defeat dedup.
//   - Drop the fragment (client-only, irrelevant to a stored destination).
//
// It returns the normalized URL, the registrable domain (host), and ok.
func NormalizeURL(raw string) (normalized, domain string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	// Require a scheme so url.Parse does not misinterpret "example.com/path"
	// as path-only (host becomes empty). We only accept http/https.
	lower := strings.ToLower(raw)
	if !(strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", false
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	if u.Port() != "" {
		if (u.Scheme == "http" && u.Port() == "80") ||
			(u.Scheme == "https" && u.Port() == "443") {
			host := u.Hostname() // hostname excludes the port
			u.Host = host
		}
	}
	if u.Path == "/" {
		u.Path = ""
	}
	// Sort query for stable dedup. We avoid re-encoding to preserve readability;
	// url.Values.Encode() sorts keys and is sufficient for our purposes.
	if u.RawQuery != "" {
		if vals, err := url.ParseQuery(u.RawQuery); err == nil {
			u.RawQuery = vals.Encode()
		}
	}

	normalized = u.String()
	return normalized, u.Hostname(), true
}

// validateURL is a thin alias used by the service; it returns the normalized
// form and domain, or ok=false when the input is not a usable http(s) URL.
func validateURL(raw string) (normalized, domain string, ok bool) {
	return NormalizeURL(raw)
}
