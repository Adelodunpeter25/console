// WebTransport server integration coverage (QUIC migration, Phase 2):
// real QUIC loopback against the wt.Server — CONNECT auth gating, CHAT_RUN
// start/attach framing, and TERMINAL spawn/echo. Unimplemented stream kinds
// (watch/tunnel/device) land with the next slice.
package tests

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	webtransport "github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// wtTestCert writes a self-signed localhost cert/key pair for the test
// server. Clients dial with InsecureSkipVerify; nothing leaves loopback.
func wtTestCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}
	certOut.Close()
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatal(err)
	}
	keyOut.Close()
	return certFile, keyFile
}

type wtHarness struct {
	server *wt.Server
	addr   string
	issuer *auth.TokenIssuer
}

func startWTTestServer(t *testing.T, runs *run.Service) wtHarness {
	t.Helper()
	certFile, keyFile := wtTestCert(t)
	issuer := auth.NewTokenIssuerWithSecret([]byte("wt-test-secret-32-bytes-!!!!"))
	ports := services.NewPortRegistry()
	t.Cleanup(ports.CloseAll)
	srv := wt.New(wt.Config{
		Mode: wt.TransportWT, WTAddr: "127.0.0.1:0",
		TLSCertFile: certFile, TLSKeyFile: keyFile,
	}, wt.Deps{
		Issuer: issuer,
		Runs:   runs,
		PTY:    services.NewPtyManager(ports, func(string) string { return "" }),
		Ports:  ports,
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("wt start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	addr := srv.Addr()
	if addr == nil {
		t.Fatal("server has no address")
	}
	return wtHarness{server: srv, addr: addr.String(), issuer: issuer}
}

func wtDial(t *testing.T, addr, token string) (*webtransport.Session, *http.Response) {
	t.Helper()
	transport := &webtransport.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // loopback test cert
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	url := "https://" + addr + "/wt/v1/connect"
	if token != "" {
		url += "?token=" + token
	}
	resp, sess, err := transport.Dial(ctx, url, nil)
	if err != nil {
		if resp != nil {
			return nil, resp
		}
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = sess.CloseWithError(0, "") })
	return sess, resp
}

func wtMint(t *testing.T, h wtHarness, device string) string {
	t.Helper()
	token, _, err := h.issuer.Mint(device, 0)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// wtOpenStream opens a stream, writes its header plus one request frame, and
// returns the open stream for reading.
func wtOpenStream(t *testing.T, sess *webtransport.Session, kind consolev1.WtStreamKind, id string, reqType string, reqPayload []byte) *webtransport.Stream {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	str, err := sess.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if err := wt.WriteStreamHeader(str, kind, id); err != nil {
		t.Fatalf("stream header: %v", err)
	}
	frame, err := wt.EncodeFrame(&consolev1.WtFrame{Type: reqType, Payload: reqPayload})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := str.Write(frame); err != nil {
		t.Fatalf("request frame: %v", err)
	}
	return str
}

// wtReadFrames collects frames until the stream closes or the deadline hits.
func wtReadFrames(t *testing.T, str *webtransport.Stream, timeout time.Duration) []*consolev1.WtFrame {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var out []*consolev1.WtFrame
	for {
		_ = str.SetReadDeadline(deadline)
		frame, err := wt.DecodeFrame(str)
		if err != nil {
			break
		}
		out = append(out, frame)
		if time.Now().After(deadline) {
			break
		}
	}
	return out
}

func frameTypes(frames []*consolev1.WtFrame) []string {
	types := make([]string, 0, len(frames))
	for _, f := range frames {
		types = append(types, f.GetType())
	}
	return types
}

func TestWTConnectRejectsMissingAndBadToken(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))

	if _, resp := wtDial(t, h.addr, ""); resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token should 401, got %v", resp)
	}
	if _, resp := wtDial(t, h.addr, "ct1_bogus.bogus"); resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token should 401, got %v", resp)
	}
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("valid token should connect")
	}
}

func mockRunService(t *testing.T, turns ...func() []loop.Event) (*run.Service, string) {
	t.Helper()
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	svc := run.NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: turns}, nil
	}
	return svc, header.ID
}

func TestWTChatRunStreamsFrames(t *testing.T) {
	svc, sessionID := mockRunService(t, func() []loop.Event {
		return []loop.Event{{Kind: loop.EventText, Text: "done"}}
	})
	h := startWTTestServer(t, svc)
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}

	req, _ := protojson.Marshal(&consolev1.WtChatRunRequest{
		SessionId: sessionID,
		Request:   &consolev1.WtChatRunRequest_Start{Start: &consolev1.WtChatRunStart{Text: "hi"}},
	})
	str := wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_CHAT_RUN, sessionID, "start", req)
	frames := wtReadFrames(t, str, 20*time.Second)

	var sawPart, sawEnd bool
	var lastSeq uint64
	for _, f := range frames {
		if f.GetSeq() > lastSeq {
			lastSeq = f.GetSeq()
		}
		switch f.GetType() {
		case "modelStreamPart":
			sawPart = true
			if f.GetSeq() == 0 {
				t.Fatal("chat frames must carry a resume sequence")
			}
			var body struct {
				Part json.RawMessage `json:"part"`
			}
			if err := json.Unmarshal(f.GetPayload(), &body); err != nil || len(body.Part) == 0 {
				t.Fatalf("part payload: %s %v", f.GetPayload(), err)
			}
		case "sessionEnd":
			sawEnd = true
		case "error":
			t.Fatalf("unexpected error frame: %s", f.GetPayload())
		}
	}
	if !sawPart {
		t.Fatalf("no modelStreamPart frames, got %v", frameTypes(frames))
	}
	if !sawEnd {
		t.Fatalf("no sessionEnd frame, got %v", frameTypes(frames))
	}
}

func TestWTChatAttachReplaysFromSince(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	svc, sessionID := mockRunService(t, func() []loop.Event {
		entered <- struct{}{}
		<-release
		return []loop.Event{{Kind: loop.EventText, Text: "done"}}
	})
	h := startWTTestServer(t, svc)

	// Start the run directly so it is in flight while we attach over WT.
	prompt := run.Prompt{Text: "slow", Provider: "mock", ModelID: "m"}
	hub, err := svc.StartRun(sessionID, prompt)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("run never started")
	}

	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	var zero int64
	req, err := protojson.Marshal(&consolev1.WtChatRunRequest{
		SessionId: sessionID,
		Request:   &consolev1.WtChatRunRequest_Attach{Attach: &consolev1.WtChatRunAttach{Since: &zero}},
	})
	if err != nil {
		t.Fatal(err)
	}
	str := wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_CHAT_RUN, sessionID, "attach", req)

	collected := make(chan []*consolev1.WtFrame, 1)
	go func() {
		collected <- wtReadFrames(t, str, 20*time.Second)
	}()
	close(release)
	helpers.WaitSettled(t, hub)

	var frames []*consolev1.WtFrame
	select {
	case frames = <-collected:
	case <-time.After(25 * time.Second):
		t.Fatal("attach stream never closed")
	}
	sawPart, sawEnd := false, false
	for _, f := range frames {
		switch f.GetType() {
		case "modelStreamPart":
			sawPart = true
		case "sessionEnd":
			sawEnd = true
		case "error":
			t.Fatalf("unexpected error frame: %s", f.GetPayload())
		}
	}
	if !sawPart || !sawEnd {
		t.Fatalf("attach must replay and follow to end, got %v", frameTypes(frames))
	}
}

func TestWTChatStartRequiresPrompt(t *testing.T) {
	svc, sessionID := mockRunService(t)
	h := startWTTestServer(t, svc)
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	req, err := protojson.Marshal(&consolev1.WtChatRunRequest{
		SessionId: sessionID,
		Request:   &consolev1.WtChatRunRequest_Start{Start: &consolev1.WtChatRunStart{Text: "  "}},
	})
	if err != nil {
		t.Fatal(err)
	}
	str := wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_CHAT_RUN, sessionID, "start", req)
	frames := wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("blank prompt must answer one error frame, got %v", frameTypes(frames))
	}
}

func TestWTTerminalSpawnsAndEchoes(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	cwd, err := os.MkdirTemp("", "wt-term-")
	if err != nil {
		t.Fatal(err)
	}
	shell := "/bin/sh"
	spawn, err := protojson.Marshal(&consolev1.WtTerminalRequest{Cwd: cwd, Shell: &shell})
	if err != nil {
		t.Fatal(err)
	}
	str := wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_TERMINAL, "", "spawn", spawn)

	// First frame must be the spawned announcement.
	_ = str.SetReadDeadline(time.Now().Add(10 * time.Second))
	first, err := wt.DecodeFrame(str)
	if err != nil {
		t.Fatalf("spawned frame: %v", err)
	}
	if first.GetType() != "spawned" {
		t.Fatalf("first frame must be spawned, got %q: %s", first.GetType(), first.GetPayload())
	}

	// Drive one command through and expect its output back.
	marker := "wt-echo-marker-123"
	input, err := wt.EncodeFrame(&consolev1.WtFrame{Type: "input", Payload: []byte("echo " + marker + "\n")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := str.Write(input); err != nil {
		t.Fatalf("input write: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	var output bytes.Buffer
	for {
		_ = str.SetReadDeadline(deadline)
		frame, err := wt.DecodeFrame(str)
		if err != nil {
			break
		}
		if frame.GetType() == "output" {
			output.Write(frame.GetPayload())
		}
		if strings.Contains(output.String(), marker) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
	}
	if !strings.Contains(output.String(), marker) {
		t.Fatalf("terminal echo missing marker, got %q", output.String())
	}
}
