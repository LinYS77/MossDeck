package bookmark

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		norm   string
		domain string
		ok     bool
	}{
		{"https basic", "https://Example.COM/Path", "https://example.com/Path", "example.com", true},
		{"trailing slash root collapses", "https://example.com/", "https://example.com", "example.com", true},
		{"keeps meaningful path", "https://example.com/a/b/", "https://example.com/a/b/", "example.com", true},
		{"default https port dropped", "https://example.com:443/x", "https://example.com/x", "example.com", true},
		{"default http port dropped", "http://example.com:80/x", "http://example.com/x", "example.com", true},
		{"nondefault port kept", "https://example.com:8443/x", "https://example.com:8443/x", "example.com", true},
		{"fragment dropped", "https://example.com/x#frag", "https://example.com/x", "example.com", true},
		{"query sorted", "https://example.com/x?b=2&a=1", "https://example.com/x?a=1&b=2", "example.com", true},
		{"scheme lowercase", "HTTPS://Example.com/X", "https://example.com/X", "example.com", true},
		{"scheme ftp rejected", "ftp://example.com/x", "", "", false},
		{"no scheme rejected", "example.com/x", "", "", false},
		{"empty rejected", "   ", "", "", false},
		{"garbage rejected", "not a url :::", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			norm, domain, ok := NormalizeURL(c.in)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v (norm=%q)", ok, c.ok, norm)
			}
			if !ok {
				return
			}
			if norm != c.norm {
				t.Errorf("normalized = %q, want %q", norm, c.norm)
			}
			if domain != c.domain {
				t.Errorf("domain = %q, want %q", domain, c.domain)
			}
		})
	}
}
