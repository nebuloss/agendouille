package auth

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ClientIP is the address a request came from: the TCP peer, or, when the
// peer is a trusted proxy, the rightmost address in X-Forwarded-For that
// is not one. Forwarded headers from anyone else are ignored: they are
// whatever the client wrote.
func ClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer := remoteAddr(r.RemoteAddr)
	if !peer.IsValid() || !inAny(peer, trusted) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return peer // a malformed chain: trust nothing in it
		}
		a = a.Unmap()
		if !inAny(a, trusted) {
			return a
		}
	}
	return peer
}

func remoteAddr(s string) netip.Addr {
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		host = s
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

func inAny(a netip.Addr, ps []netip.Prefix) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// SameOrigin reports whether a state-changing browser request comes from
// our own pages: Sec-Fetch-Site says so, or, from a browser too old to
// send it, Origin is publicURL's origin (the request's Host when empty).
// A request with neither header did not come from a browser, so a cookie
// it carries was not sent behind its owner's back.
func SameOrigin(r *http.Request, publicURL string) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "same-site", "cross-site":
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	o, err := url.Parse(origin)
	if err != nil || o.Host == "" {
		return false
	}
	if publicURL != "" {
		if u, err := url.Parse(publicURL); err == nil {
			return strings.EqualFold(o.Scheme, u.Scheme) && strings.EqualFold(o.Host, u.Host)
		}
	}
	return strings.EqualFold(o.Host, r.Host)
}

type principalKey struct{}

// WithPrincipal puts p in ctx for the handlers behind a check.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// ContextPrincipal is the principal a check put in ctx, or nil.
func ContextPrincipal(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}
