//go:build iroh && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

// The iroh backend end to end: a client dials the server by node ID alone
// and carries a real HTTP request over a QUIC stream.
package tests

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	iroh "github.com/Adelodunpeter25/iroh-go"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/irohbackend"
)

func startIroh(t *testing.T, dir, target string) *irohbackend.Manager {
	t.Helper()
	ready := make(chan string, 1)
	m := &irohbackend.Manager{Dir: dir, Target: target, OnReady: func(id string) { ready <- id }}
	go func() { _ = m.Start(context.Background()) }()
	select {
	case <-ready:
	case <-time.After(15 * time.Second):
		t.Fatal("iroh manager never became ready")
	}
	return m
}

func irohGet(t *testing.T, nodeID, request string) *http.Response {
	t.Helper()
	preset := iroh.PresetN0()
	ep, err := iroh.EndpointBind(iroh.EndpointOptions{Preset: &preset, Alpns: &[][]byte{[]byte(irohbackend.ALPN)}})
	if err != nil {
		t.Fatal("step 1:", iroh.As(err))
	}
	ep.Online()
	id, err := iroh.EndpointIdFromString(nodeID)
	if err != nil {
		t.Fatal("step 2:", iroh.As(err))
	}
	conn, err := ep.Connect(iroh.NewEndpointAddr(id, nil, nil), []byte(irohbackend.ALPN))
	if err != nil {
		t.Fatal("step 3:", iroh.As(err))
	}
	stream, err := conn.OpenBi()
	if err != nil {
		t.Fatal("step 4:", iroh.As(err))
	}
	if err := stream.Send().WriteAll([]byte(request)); err != nil {
		t.Fatal("step 5:", iroh.As(err))
	}
	if err := stream.Send().Finish(); err != nil {
		t.Fatal("step 6:", iroh.As(err))
	}
	body, err := stream.Recv().ReadToEnd(1 << 20)
	if err != nil {
		t.Fatal("step 7:", iroh.As(err))
	}
	resp, err := http.ReadResponse(bufio.NewReader(strings.NewReader(string(body))), nil)
	if err != nil {
		t.Fatalf("bad response %q: %v", body, err)
	}
	return resp
}

func TestIrohForwardsHTTPByNodeID(t *testing.T) {
	dir := t.TempDir()
	m := startIroh(t, dir, localServer(t))

	if info, ok := irohbackend.ReadInfo(dir); !ok || info.NodeID != m.NodeID() {
		t.Fatalf("stored info = %+v, %v; node = %s", info, ok, m.NodeID())
	}

	resp := irohGet(t, m.NodeID(), "POST /api/x?y=1 HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nConnection: close\r\n\r\nhello")
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(b) != "POST:hello" || resp.Header.Get("X-Seen-Path") != "/api/x?y=1" {
		t.Fatalf("response = %d %q %v", resp.StatusCode, b, resp.Header)
	}
}

func TestIrohNodeIDSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	m := startIroh(t, dir, localServer(t))
	first := m.NodeID()
	key, _ := irohbackend.Store{Dir: dir}.LoadKey()
	if len(key) != 32 || first == "" {
		t.Fatalf("key=%d bytes, node=%q", len(key), first)
	}
	// A second manager on the same dir derives the same identity.
	m2 := startIroh(t, dir, localServer(t))
	if m2.NodeID() != first {
		t.Fatalf("node ID changed across restart: %s -> %s", first, m2.NodeID())
	}
}
