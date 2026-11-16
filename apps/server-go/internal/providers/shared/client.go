package shared

import (
	"net"
	"net/http"
	"time"
)

// Transport is the tuned HTTP transport shared across LLM providers,
// connection reuse, keep-alives, and usage scrapers.
var Transport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   20,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// Client is a pooled HTTP client with no timeout, suitable for long-lived
// streaming turns. Context cancellation controls request lifecycle.
var Client = &http.Client{
	Transport: Transport,
	Timeout:   0,
}

// PooledClient returns an http.Client backed by Transport with the given timeout.
func PooledClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: Transport,
		Timeout:   timeout,
	}
}
