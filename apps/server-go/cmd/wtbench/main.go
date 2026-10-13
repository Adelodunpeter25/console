// wtbench exercises WebTransport fanout against a running server (transport
// benchmarking, not harness token cost — see cmd/bench for that).
//
// Modes:
//
//	open    open N watch streams, report open-to-first-frame latency
//	tunnel  pump SIZE bytes through one tunnel stream, report MB/s
//	        (bench and server must share loopback: the echo target runs here)
//	blast   open N notification watches, push COUNT notifications via the
//	        debug endpoint, report delivery latency and loss
//
// Every mode prints human lines, or one JSON summary with --json.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	webtransport "github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:443", "WT server UDP address")
	rest := flag.String("rest", "http://127.0.0.1:3000", "REST base URL for token-adjacent calls (debug snapshots)")
	token := flag.String("token", "", "API bearer token (mint via POST /api/auth/token)")
	mode := flag.String("mode", "open", "open | tunnel | blast")
	n := flag.Int("n", 500, "streams to open (open/blast)")
	count := flag.Int("count", 50, "notifications to push (blast)")
	size := flag.Int64("size", 100<<20, "bytes to pump (tunnel)")
	asJSON := flag.Bool("json", false, "emit one JSON summary instead of human lines")
	flag.Parse()
	if *token == "" {
		fmt.Fprintln(os.Stderr, "wtbench: -token is required")
		os.Exit(2)
	}
	summary, err := run(*addr, *rest, *token, *mode, *n, *count, *size)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wtbench:", err)
		os.Exit(1)
	}
	if *asJSON {
		raw, _ := json.Marshal(summary)
		fmt.Println(string(raw))
		return
	}
	for _, line := range summary.human {
		fmt.Println(line)
	}
}

type summary struct {
	human []string
	extra map[string]any
}

func (s *summary) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range s.extra {
		out[k] = v
	}
	return json.Marshal(out)
}

func dial(addr, token string, timeout time.Duration) (*webtransport.Session, time.Duration, error) {
	transport := &webtransport.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // bench tool, explicit flag surface
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	resp, sess, err := transport.Dial(ctx, "https://"+addr+"/wt/v1/connect?token="+token, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, 0, fmt.Errorf("dial status=%d: %w", status, err)
	}
	return sess, time.Since(start), nil
}

func restClient(rest string) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // bench tool, explicit flag surface
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

func debugSnapshot(rest, token string) map[string]any {
	req, _ := http.NewRequest("GET", strings.TrimSuffix(rest, "/")+"/api/debug/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := restClient(rest).Do(req)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return out.Data
}

func quantiles(durations []time.Duration) (p50, p99 time.Duration) {
	if len(durations) == 0 {
		return 0, 0
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	return durations[len(durations)/2], durations[int(float64(len(durations))*0.99)]
}

func run(addr, rest, token, mode string, n, count int, size int64) (*summary, error) {
	sess, handshake, err := dial(addr, token, 60*time.Second)
	if err != nil {
		return nil, err
	}
	defer sess.CloseWithError(0, "")
	serverBefore := debugSnapshot(rest, token)
	switch mode {
	case "open":
		return runOpen(sess, rest, token, handshake, n, serverBefore)
	case "tunnel":
		return runTunnel(sess, rest, token, handshake, size, serverBefore)
	case "blast":
		return runBlast(sess, rest, token, handshake, n, count, serverBefore)
	default:
		return nil, fmt.Errorf("unknown mode %q (open | tunnel | blast)", mode)
	}
}

func serverDelta(rest, token string, before map[string]any) map[string]any {
	after := debugSnapshot(rest, token)
	delta := map[string]any{}
	for _, key := range []string{"heapAlloc", "heapSys", "goroutines", "sessionsTotal"} {
		b, _ := before[key].(float64)
		a, _ := after[key].(float64)
		delta[key+"Delta"] = a - b
	}
	delta["after"] = after
	return delta
}

func serverHumanLine(delta map[string]any) string {
	mb := func(v any) string {
		f, _ := v.(float64)
		return fmt.Sprintf("%+.1fMB", f/(1024*1024))
	}
	return fmt.Sprintf("server: heap %s, goroutines %+.0f, sessionsTotal %+.0f",
		mb(delta["heapAllocDelta"]), delta["goroutinesDelta"], delta["sessionsTotalDelta"])
}

func runOpen(sess *webtransport.Session, rest, token string, handshake time.Duration, n int, serverBefore map[string]any) (*summary, error) {
	// Ports watches answer an immediate snapshot frame, so open-to-first-
	// frame is a full client→server→client round trip per stream.
	req, err := protojson.Marshal(&consolev1.WtWatchRequest{Target: consolev1.WtWatchTarget_WT_WATCH_TARGET_PORTS})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	latencies := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		openStart := time.Now()
		str, err := sess.OpenStreamSync(ctx)
		if err != nil {
			return nil, fmt.Errorf("open stream %d: %w", i, err)
		}
		if err := wt.WriteStreamHeader(str, consolev1.WtStreamKind_WT_STREAM_KIND_WATCH, ""); err != nil {
			return nil, fmt.Errorf("stream %d header: %w", i, err)
		}
		frame, err := wt.EncodeFrame(&consolev1.WtFrame{Type: "watch", Payload: req})
		if err != nil {
			return nil, err
		}
		if _, err := str.Write(frame); err != nil {
			return nil, fmt.Errorf("stream %d request: %w", i, err)
		}
		_ = str.SetReadDeadline(time.Now().Add(30 * time.Second))
		first, err := wt.DecodeFrame(str)
		if err != nil {
			return nil, fmt.Errorf("stream %d first frame: %w", i, err)
		}
		if first.GetType() != "ports" {
			return nil, fmt.Errorf("stream %d: want ports snapshot, got %q", i, first.GetType())
		}
		latencies = append(latencies, time.Since(openStart))
	}
	p50, p99 := quantiles(latencies)
	extra := map[string]any{
		"mode": "open", "streams": n,
		"handshakeMs":           handshake.Seconds() * 1000,
		"openToFirstFrameP50Ms": p50.Seconds() * 1000,
		"openToFirstFrameP99Ms": p99.Seconds() * 1000,
	}
	serverSummary := serverDelta(rest, token, serverBefore)
	for k, v := range serverSummary {
		extra[k] = v
	}
	return &summary{
		human: []string{
			fmt.Sprintf("handshake: %s", handshake.Round(time.Millisecond)),
			fmt.Sprintf("streams: %d open-to-first-frame p50=%s p99=%s", n, p50, p99),
			serverHumanLine(serverSummary),
		},
		extra: extra,
	}, nil
}

func runTunnel(sess *webtransport.Session, rest, token string, handshake time.Duration, size int64, serverBefore map[string]any) (*summary, error) {
	// Local echo target: the tunneled TCP leg terminates here, so this
	// mode only measures colocated servers.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				buf := make([]byte, 64*1024)
				for {
					n, err := conn.Read(buf)
					if n > 0 {
						if _, err := conn.Write(buf[:n]); err != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	req, err := protojson.Marshal(&consolev1.WtTunnelRequest{Port: port})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	str, err := sess.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open tunnel: %w", err)
	}
	if err := wt.WriteStreamHeader(str, consolev1.WtStreamKind_WT_STREAM_KIND_PORT_TUNNEL, ""); err != nil {
		return nil, err
	}
	frame, err := wt.EncodeFrame(&consolev1.WtFrame{Type: "tunnel", Payload: req})
	if err != nil {
		return nil, err
	}
	if _, err := str.Write(frame); err != nil {
		return nil, fmt.Errorf("tunnel request: %w", err)
	}
	// Deterministic pattern: byte i carries byte(i), verified on return.
	const chunk = 64 * 1024
	sent := make([]byte, chunk)
	var received int64
	verify := true
	start := time.Now()
	_ = str.SetReadDeadline(time.Now().Add(5 * time.Minute))
	var offset int64
	for offset < size {
		n := chunk
		if remaining := size - offset; remaining < int64(n) {
			n = int(remaining)
		}
		for i := 0; i < n; i++ {
			sent[i] = byte((offset + int64(i)) & 0xff)
		}
		if _, err := str.Write(sent[:n]); err != nil {
			return nil, fmt.Errorf("tunnel write at %d: %w", offset, err)
		}
		got := make([]byte, n)
		if _, err := io.ReadFull(str, got); err != nil {
			return nil, fmt.Errorf("tunnel read at %d: %w", offset, err)
		}
		for i := 0; i < n; i++ {
			if got[i] != sent[i] {
				verify = false
				break
			}
		}
		if !verify {
			break
		}
		offset += int64(n)
		received += int64(n)
	}
	elapsed := time.Since(start)
	mbps := float64(received) / elapsed.Seconds() / (1024 * 1024)
	extra := map[string]any{
		"mode": "tunnel", "bytes": received,
		"handshakeMs": handshake.Seconds() * 1000,
		"seconds":     elapsed.Seconds(), "mbPerSec": mbps, "verified": verify,
	}
	serverSummary := serverDelta(rest, token, serverBefore)
	for k, v := range serverSummary {
		extra[k] = v
	}
	return &summary{
		human: []string{
			fmt.Sprintf("handshake: %s", handshake.Round(time.Millisecond)),
			fmt.Sprintf("tunnel: %d bytes in %s = %.1f MB/s, verified=%v", received, elapsed.Round(time.Millisecond), mbps, verify),
			serverHumanLine(serverSummary),
		},
		extra: extra,
	}, nil
}

func runBlast(sess *webtransport.Session, rest, token string, handshake time.Duration, n, count int, serverBefore map[string]any) (*summary, error) {
	req, err := protojson.Marshal(&consolev1.WtWatchRequest{Target: consolev1.WtWatchTarget_WT_WATCH_TARGET_NOTIFICATIONS})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	type delivery struct {
		index int
		at    time.Time
	}
	deliveries := make(chan delivery, n*count)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		str, err := sess.OpenStreamSync(ctx)
		if err != nil {
			return nil, fmt.Errorf("open stream %d: %w", i, err)
		}
		if err := wt.WriteStreamHeader(str, consolev1.WtStreamKind_WT_STREAM_KIND_WATCH, ""); err != nil {
			return nil, err
		}
		frame, err := wt.EncodeFrame(&consolev1.WtFrame{Type: "watch", Payload: req})
		if err != nil {
			return nil, err
		}
		if _, err := str.Write(frame); err != nil {
			return nil, fmt.Errorf("stream %d request: %w", i, err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = str.SetReadDeadline(time.Now().Add(2 * time.Minute))
			for {
				frame, err := wt.DecodeFrame(str)
				if err != nil {
					return
				}
				if frame.GetType() != "notification" {
					continue
				}
				var body struct {
					Subtitle string `json:"subtitle"`
				}
				var index int
				if err := json.Unmarshal(frame.GetPayload(), &body); err == nil {
					fmt.Sscanf(body.Subtitle, "blast #%d", &index)
				}
				deliveries <- delivery{index: index, at: time.Now()}
			}
		}()
	}
	pushTimes := make([]time.Time, 0, count)
	client := restClient(rest)
	for i := 0; i < count; i++ {
		body, _ := json.Marshal(map[string]string{
			"kind": "bench", "title": "blast", "subtitle": fmt.Sprintf("blast #%d", i),
		})
		httpReq, _ := http.NewRequest("POST", strings.TrimSuffix(rest, "/")+"/api/debug/notify", bytes.NewReader(body))
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("Content-Type", "application/json")
		pushTimes = append(pushTimes, time.Now())
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("push %d: %w", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("push %d status %d", i, resp.StatusCode)
		}
	}
	// Every stream should see every notification.
	want := n * count
	latencies := make([]time.Duration, 0, want)
	got := 0
	timeout := time.After(2 * time.Minute)
	for got < want {
		select {
		case d := <-deliveries:
			if d.index >= 0 && d.index < count {
				latencies = append(latencies, d.at.Sub(pushTimes[d.index]))
			} else {
				latencies = append(latencies, 0)
			}
			got++
		case <-timeout:
			goto done
		}
	}
done:
	p50, p99 := quantiles(latencies)
	lost := want - got
	extra := map[string]any{
		"mode": "blast", "streams": n, "pushed": count,
		"handshakeMs":   handshake.Seconds() * 1000,
		"deliveries":    got,
		"lost":          lost,
		"deliveryP50Ms": p50.Seconds() * 1000,
		"deliveryP99Ms": p99.Seconds() * 1000,
	}
	serverSummary := serverDelta(rest, token, serverBefore)
	for k, v := range serverSummary {
		extra[k] = v
	}
	return &summary{
		human: []string{
			fmt.Sprintf("handshake: %s", handshake.Round(time.Millisecond)),
			fmt.Sprintf("blast: %d/%d deliveries, push-to-receive p50=%s p99=%s, lost=%d", got, want, p50, p99, lost),
			serverHumanLine(serverSummary),
		},
		extra: extra,
	}, nil
}
