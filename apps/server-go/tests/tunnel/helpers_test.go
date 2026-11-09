//go:build (linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64)

// Shared helpers for the iroh tunnel tests.
package tests

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fasthttp/websocket"
)

func localServer(t *testing.T) string {
	t.Helper()
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			c, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer c.Close()
			for {
				k, m, err := c.ReadMessage()
				if err != nil {
					return
				}
				_ = c.WriteMessage(k, m)
			}
		}
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Seen-Path", r.URL.RequestURI())
		w.Header().Set("X-Seen-Host", r.Host)
		w.Write([]byte(r.Method + ":" + string(b)))
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).String()
}
