//go:build iroh && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

// irohecho serves a tiny HTTP target behind the iroh backend and prints the
// node ID. It exists so the Rust desktop client can be tested against the
// real Go bridge.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/irohbackend"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Seen-Path", r.URL.RequestURI())
		fmt.Fprintf(w, "%s:%s", r.Method, b)
	}))
	dir, _ := os.MkdirTemp("", "irohecho")
	defer os.RemoveAll(dir)
	m := &irohbackend.Manager{Dir: dir, Target: ln.Addr().String(), OnReady: func(id string) {
		fmt.Println("NODE_ID", id)
	}}
	go m.Start(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
