// wtbench opens N WebTransport watch streams against a running server and
// reports open-to-first-frame latency plus client allocation deltas. It
// measures transport fanout, not harness token cost (see cmd/bench for
// that): `go run ./cmd/wtbench -addr 127.0.0.1:443 -token <minted>`.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	webtransport "github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:443", "WT server UDP address")
	token := flag.String("token", "", "API bearer token (mint via POST /api/auth/token)")
	streams := flag.Int("n", 500, "watch streams to open")
	flag.Parse()
	if *token == "" {
		fmt.Fprintln(os.Stderr, "wtbench: -token is required")
		os.Exit(2)
	}
	if err := run(*addr, *token, *streams); err != nil {
		fmt.Fprintln(os.Stderr, "wtbench:", err)
		os.Exit(1)
	}
}

func run(addr, token string, n int) error {
	transport := &webtransport.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // bench tool, explicit flag surface
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	start := time.Now()
	resp, sess, err := transport.Dial(ctx, "https://"+addr+"/wt/v1/connect?token="+token, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return fmt.Errorf("dial status=%d: %w", status, err)
	}
	defer sess.CloseWithError(0, "")
	fmt.Printf("handshake: %s\n", time.Since(start))

	// Ports watches answer an immediate snapshot frame, so open-to-first-
	// frame is a full client→server→client round trip per stream.
	req, err := protojson.Marshal(&consolev1.WtWatchRequest{Target: consolev1.WtWatchTarget_WT_WATCH_TARGET_PORTS})
	if err != nil {
		return err
	}
	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	goroutinesBefore := runtime.NumGoroutine()

	latencies := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		openStart := time.Now()
		str, err := sess.OpenStreamSync(ctx)
		if err != nil {
			return fmt.Errorf("open stream %d: %w", i, err)
		}
		if err := wt.WriteStreamHeader(str, consolev1.WtStreamKind_WT_STREAM_KIND_WATCH, ""); err != nil {
			return fmt.Errorf("stream %d header: %w", i, err)
		}
		frame, err := wt.EncodeFrame(&consolev1.WtFrame{Type: "watch", Payload: req})
		if err != nil {
			return err
		}
		if _, err := str.Write(frame); err != nil {
			return fmt.Errorf("stream %d request: %w", i, err)
		}
		_ = str.SetReadDeadline(time.Now().Add(30 * time.Second))
		first, err := wt.DecodeFrame(str)
		if err != nil {
			return fmt.Errorf("stream %d first frame: %w", i, err)
		}
		if first.GetType() != "ports" {
			return fmt.Errorf("stream %d: want ports snapshot, got %q", i, first.GetType())
		}
		latencies = append(latencies, time.Since(openStart))
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)/2]
	p99 := latencies[int(float64(len(latencies))*0.99)]
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	fmt.Printf("streams: %d open-to-first-frame p50=%s p99=%s\n", n, p50, p99)
	fmt.Printf("client heap delta: +%d bytes (%d/stream), goroutines %+d\n",
		int64(after.HeapAlloc)-int64(before.HeapAlloc),
		(int64(after.HeapAlloc)-int64(before.HeapAlloc))/int64(n),
		runtime.NumGoroutine()-goroutinesBefore)
	return nil
}
