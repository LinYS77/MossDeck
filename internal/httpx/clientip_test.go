package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234" by default, which we
// rely on as a stable "direct client" peer address in these tests.

func newIPReq(remoteAddr, xff, real string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr // always assign (even "") so the empty case is testable
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	if real != "" {
		req.Header.Set("X-Real-IP", real)
	}
	return req
}

func TestNewClientIPResolverRejectsInvalidCIDR(t *testing.T) {
	cases := [][]string{
		{"10.0.0.0/8", "not-a-cidr"},
		{"999.999.999.999"},
		{"10.0.0/8"},
	}
	for _, cidrs := range cases {
		if _, err := NewClientIPResolver(cidrs); err == nil {
			t.Fatalf("expected error for %v", cidrs)
		}
	}
}

func TestNewClientIPResolverAcceptsValid(t *testing.T) {
	for _, cidrs := range [][]string{
		{},
		nil,
		{"10.0.0.0/8"},
		{"10.0.0.0/8", "172.16.0.0/12"},
		{"10.0.0.5"},           // bare IPv4 -> /32
		{"::1"},                // bare IPv6 -> /128
		{"  10.0.0.0/8  ", ""}, // trimmed + blanks ignored
	} {
		r, err := NewClientIPResolver(cidrs)
		if err != nil {
			t.Fatalf("NewClientIPResolver(%v): %v", cidrs, err)
		}
		_ = r
	}
}

func TestUntrustedIgnoresForgedHeaders(t *testing.T) {
	// No trusted CIDRs: a direct client must not rotate its apparent IP by
	// forging X-Forwarded-For / X-Real-IP. This is the core security property.
	r, _ := NewClientIPResolver(nil)
	if r.Trusted() {
		t.Fatal("empty resolver must report Trusted() == false")
	}
	for _, tc := range []struct {
		name string
		xff  string
		real string
	}{
		{"forged xff", "203.0.113.7, 198.51.100.2", ""},
		{"forged real", "", "203.0.113.7"},
		{"both forged", "203.0.113.7", "198.51.100.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := r.ClientIP(newIPReq("192.0.2.1:1234", tc.xff, tc.real))
			if got != "192.0.2.1" {
				t.Fatalf("untrusted resolver honored a forged header: got %q, want peer 192.0.2.1", got)
			}
		})
	}
}

func TestTrustedPeerHonorsHeaders(t *testing.T) {
	// Trust the reverse proxy's network. When the TCP peer is inside it, the
	// forwarded headers are trusted.
	r, _ := NewClientIPResolver([]string{"10.0.0.0/8"})

	t.Run("xff leftmost wins", func(t *testing.T) {
		req := newIPReq("10.0.0.1:4000", "203.0.113.9, 10.0.0.1", "")
		if got := r.ClientIP(req); got != "203.0.113.9" {
			t.Fatalf("got %q, want 203.0.113.9", got)
		}
	})

	t.Run("x-real-ip fallback", func(t *testing.T) {
		req := newIPReq("10.0.0.1:4000", "", "198.51.100.4")
		if got := r.ClientIP(req); got != "198.51.100.4" {
			t.Fatalf("got %q, want 198.51.100.4", got)
		}
	})
}

func TestTrustedPeerButBadHeadersFallsBackToPeer(t *testing.T) {
	// Trusted peer, but the proxy did not set a usable IP (empty/garbage):
	// fall back to the peer (the proxy) rather than trusting junk.
	r, _ := NewClientIPResolver([]string{"10.0.0.0/8"})
	for _, tc := range []struct {
		name string
		xff  string
		real string
	}{
		{"empty headers", "", ""},
		{"garbage xff", "not-an-ip, 1.2.3.4", ""},
		{"garbage real", "", "definitely-not-an-ip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := r.ClientIP(newIPReq("10.0.0.1:4000", tc.xff, tc.real))
			if got != "10.0.0.1" {
				t.Fatalf("got %q, want peer 10.0.0.1", got)
			}
		})
	}
}

func TestUntrustedPeerEvenWhenSomeCidrsConfigured(t *testing.T) {
	// CIDRs are configured, but the actual peer is NOT inside them: headers
	// must still be ignored (a client from the open internet cannot trust itself).
	r, _ := NewClientIPResolver([]string{"10.0.0.0/8"})
	if !r.Trusted() {
		t.Fatal("expected Trusted() == true with a configured CIDR")
	}
	got := r.ClientIP(newIPReq("203.0.113.50:5000", "10.0.0.99", ""))
	if got != "203.0.113.50" {
		t.Fatalf("got %q, want peer 203.0.113.50 (untrusted peer ignores headers)", got)
	}
}

func TestBareIPIsTrustedAsHost(t *testing.T) {
	r, _ := NewClientIPResolver([]string{"10.0.0.5"}) // single host
	got := r.ClientIP(newIPReq("10.0.0.5:4000", "203.0.113.1", ""))
	if got != "203.0.113.1" {
		t.Fatalf("bare-IP trust: got %q, want 203.0.113.1", got)
	}
	// A different host in the same /24 is NOT trusted (bare IP = /32).
	got = r.ClientIP(newIPReq("10.0.0.6:4000", "203.0.113.1", ""))
	if got != "10.0.0.6" {
		t.Fatalf("sibling host should be untrusted: got %q, want 10.0.0.6", got)
	}
}

func TestNilRequest(t *testing.T) {
	r, _ := NewClientIPResolver(nil)
	if got := r.ClientIP(nil); got != "unknown" {
		t.Fatalf("nil request: got %q, want unknown", got)
	}
}

func TestEmptyRemoteAddr(t *testing.T) {
	r, _ := NewClientIPResolver(nil)
	if got := r.ClientIP(newIPReq("", "", "")); got != "unknown" {
		t.Fatalf("empty remote addr: got %q, want unknown", got)
	}
}
