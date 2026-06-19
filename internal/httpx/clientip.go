// Package httpx holds small, reusable HTTP helpers shared across handlers and
// middleware. Currently it provides ClientIPResolver, which resolves the real
// client IP behind a trusted reverse proxy WITHOUT allowing header spoofing by
// clients that connect directly or via an untrusted hop.
package httpx

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ClientIPResolver resolves the real client IP of an HTTP request.
//
// Trust model (the public-safety core):
//
//   - If trusted CIDRs are configured AND the request's TCP peer (RemoteAddr
//     host) is within one of them, forwarded headers set by that proxy are
//     honoured — preferring the leftmost X-Forwarded-For entry, then
//     X-Real-IP — because a trusted reverse proxy overwrites them.
//   - Otherwise the resolver returns the TCP peer host only and ignores any
//     X-Forwarded-For / X-Real-IP a direct or untrusted client may have set.
//
// This is what makes per-IP rate limiting (and future audit/access control)
// safe to expose publicly: a client cannot forge a header to rotate its
// apparent address unless it is connecting from a network you already trust.
type ClientIPResolver struct {
	// trustedNets are the CIDRs whose forwarded headers we trust.
	// nil/empty means "trust no proxy": every request's IP is its TCP peer.
	trustedNets []*net.IPNet
}

// NewClientIPResolver parses cidrs and returns a resolver that trusts
// forwarded headers only from peers within those networks. Each entry must be
// a valid CIDR (IPv4/IPv6); a bare IP is accepted as its single-host network
// (/32 or /128). Any unparseable entry yields an error so misconfiguration is
// surfaced at startup rather than silently weakening security.
func NewClientIPResolver(cidrs []string) (*ClientIPResolver, error) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			// Tolerate a bare IP by treating it as a single-host network.
			if ip := net.ParseIP(c); ip != nil {
				ipnet = singleHostIPNet(ip)
			} else {
				return nil, fmt.Errorf("httpx: invalid trusted proxy CIDR %q: %w", c, err)
			}
		}
		nets = append(nets, ipnet)
	}
	return &ClientIPResolver{trustedNets: nets}, nil
}

// Trusted reports whether any trusted-proxy CIDR is configured. When false,
// forwarded headers are never honoured, so apparent IPs are always TCP peers.
func (r *ClientIPResolver) Trusted() bool { return len(r.trustedNets) > 0 }

// ClientIP resolves the real client IP for the request according to the trust
// model documented on the type. It returns "unknown" when no IP can be
// determined.
func (r *ClientIPResolver) ClientIP(req *http.Request) string {
	if req == nil {
		return "unknown"
	}
	peer := peerHost(req.RemoteAddr)
	if r.Trusted() && r.peerTrusted(peer) {
		if ip := firstForwarded(req); ip != "" {
			return ip
		}
	}
	if peer == "" {
		return "unknown"
	}
	return peer
}

// peerTrusted reports whether peer is within any trusted CIDR.
func (r *ClientIPResolver) peerTrusted(peer string) bool {
	ip := net.ParseIP(peer)
	if ip == nil {
		return false
	}
	for _, n := range r.trustedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// firstForwarded extracts the original client IP from forwarded headers,
// preferring the leftmost X-Forwarded-For entry and falling back to X-Real-IP.
// It returns "" when no valid-looking IP is present. net.ParseIP guards
// against malformed/garbage values reaching downstream consumers.
func firstForwarded(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// XFF is comma-separated; the leftmost entry is the original client
		// (a trusted proxy appends subsequent hops on the right).
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}
	if ip := strings.TrimSpace(req.Header.Get("X-Real-IP")); ip != "" && net.ParseIP(ip) != nil {
		return ip
	}
	return ""
}

// peerHost extracts the host portion of an addr of the form "host:port".
// For inputs without a port (e.g. an already-bare IP) it returns the input
// unchanged; for an empty input it returns "".
func peerHost(remoteAddr string) string {
	if remoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// singleHostIPNet turns a bare IP into a /32 (v4) or /128 (v6) network so it
// can be used uniformly with net.IPNet.Contains.
func singleHostIPNet(ip net.IP) *net.IPNet {
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
}
