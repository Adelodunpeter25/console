package hostc

import (
	"net/http"
	"net/url"
	"strings"
)

// hopByHopHeaders are connection-level headers a proxy must never forward
// (RFC 9110 section 7.6.1).
var hopByHopHeaders = []string{
	"connection",
	"keep-alive",
	"proxy-authenticate",
	"proxy-authorization",
	"proxy-connection",
	"te",
	"trailer",
	"transfer-encoding",
	"upgrade",
}

// webSocketHandshakeHeaders are negotiated per connection, so neither
// direction of a handshake forwards them.
var webSocketHandshakeHeaders = []string{
	"sec-websocket-accept",
	"sec-websocket-extensions",
	"sec-websocket-key",
	"sec-websocket-protocol",
	"sec-websocket-version",
}

// stripHopByHop drops hop-by-hop headers, the headers named by the
// Connection header, and any extra names given.
func stripHopByHop(headers HeaderList, drop ...string) HeaderList {
	dropped := make(map[string]bool, len(drop))
	for _, name := range drop {
		dropped[strings.ToLower(name)] = true
	}
	for _, h := range headers {
		if strings.EqualFold(h[0], "connection") {
			for _, token := range strings.Split(h[1], ",") {
				dropped[strings.ToLower(strings.TrimSpace(token))] = true
			}
		}
	}
	out := make(HeaderList, 0, len(headers))
	for _, h := range headers {
		lower := strings.ToLower(h[0])
		if containsString(hopByHopHeaders, lower) || dropped[lower] {
			continue
		}
		out = append(out, h)
	}
	return out
}

// stripWebSocketHandshake drops the per-connection handshake headers.
func stripWebSocketHandshake(headers HeaderList) HeaderList {
	return stripHopByHop(headers, webSocketHandshakeHeaders...)
}

// toLocalRequestHeaders makes a public request look like it was made to the
// local server directly, so dev servers that check Host or Origin accept it.
func toLocalRequestHeaders(headers HeaderList, publicOrigin string, target *url.URL) HeaderList {
	out := make(HeaderList, 0, len(headers)+1)
	out = append(out, [2]string{"host", target.Host})
	localOrigin := originOf(target)
	for _, h := range headers {
		lower := strings.ToLower(h[0])
		if lower == "host" {
			continue
		}
		if (lower == "origin" || lower == "referer") && strings.HasPrefix(h[1], publicOrigin) {
			out = append(out, [2]string{h[0], localOrigin + strings.TrimPrefix(h[1], publicOrigin)})
			continue
		}
		out = append(out, h)
	}
	return out
}

// toPublicResponseHeaders points redirects at the local server back at the
// public URL.
func toPublicResponseHeaders(headers HeaderList, publicOrigin string, target *url.URL) HeaderList {
	origins := localOrigins(target)
	out := make(HeaderList, 0, len(headers))
	for _, h := range headers {
		if !strings.EqualFold(h[0], "location") {
			out = append(out, h)
			continue
		}
		value := h[1]
		for _, origin := range origins {
			if value == origin || strings.HasPrefix(value, origin+"/") || strings.HasPrefix(value, origin+"?") {
				value = publicOrigin + strings.TrimPrefix(value, origin)
				break
			}
		}
		out = append(out, [2]string{h[0], value})
	}
	return out
}

// localOrigins lists the origins a local server may redirect to.
// http://localhost:3000 is also reachable as 127.0.0.1 and [::1], and apps
// redirect to any of them.
func localOrigins(target *url.URL) []string {
	origins := []string{originOf(target)}
	host := target.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		for _, alt := range []string{"localhost", "127.0.0.1", "[::1]"} {
			origins = append(origins, target.Scheme+"://"+alt+portSuffix(target))
		}
	}
	return origins
}

// originOf returns the scheme://host[:port] prefix of a URL.
func originOf(u *url.URL) string {
	return u.Scheme + "://" + u.Host
}

// portSuffix returns the ":port" part of a URL's authority, if it has one.
func portSuffix(u *url.URL) string {
	if u.Port() == "" {
		return ""
	}
	return ":" + u.Port()
}

// headerList converts response headers into the wire form.
func headerList(h http.Header) HeaderList {
	out := make(HeaderList, 0, len(h))
	for name, values := range h {
		for _, value := range values {
			out = append(out, [2]string{name, value})
		}
	}
	return out
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
