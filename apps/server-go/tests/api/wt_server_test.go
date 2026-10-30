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
	"fmt"
	gitsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/git"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"io"
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
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
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
	server        *wt.Server
	addr          string
	issuer        *auth.TokenIssuer
	watch         *fs.FsWatchService
	git           *gitsvc.GitService
	notifications *services.NotificationService
	scripts       *projectssvc.ProjectScriptsService
	projects      *projectssvc.ProjectService
	ports         *terminalsvc.PortRegistry
	devices       *services.DeviceService
}

func startWTTestServer(t *testing.T, runs *run.Service) wtHarness {
	t.Helper()
	certFile, keyFile := wtTestCert(t)
	issuer := auth.NewTokenIssuerWithSecret([]byte("wt-test-secret-32-bytes-!!!!"))
	ports := terminalsvc.NewPortRegistry()
	t.Cleanup(ports.CloseAll)
	watch, err := fs.NewFsWatchService()
	if err != nil {
		t.Fatalf("watch service: %v", err)
	}
	t.Cleanup(watch.Close)
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	projects := projectssvc.NewProjectService(manager)
	notifications := services.NewNotificationService()
	git := gitsvc.NewGitService()
	scripts := projectssvc.NewProjectScriptsService(projects, ports)
	devices := services.NewDeviceService()
	t.Cleanup(func() { _ = devices.Close() })
	srv := wt.New(wt.Config{
		Mode: wt.TransportWT, WTAddr: "127.0.0.1:0",
		TLSCertFile: certFile, TLSKeyFile: keyFile,
	}, wt.Deps{
		Issuer:        issuer,
		Runs:          runs,
		PTY:           terminalsvc.NewPtyManager(ports, func(string) string { return "" }),
		Ports:         ports,
		Watch:         watch,
		Git:           git,
		Notifications: notifications,
		Scripts:       scripts,
		Devices:       devices,
	})
	if err := srv.Start(); err != nil {
		t.Fatalf("wt start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	addr := srv.Addr()
	if addr == nil {
		t.Fatal("server has no address")
	}
	return wtHarness{server: srv, addr: addr.String(), issuer: issuer, watch: watch, git: git, notifications: notifications, scripts: scripts, projects: projects, ports: ports, devices: devices}
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

// wtOpenWatch opens a WATCH stream for one target and returns it open.
func wtOpenWatch(t *testing.T, sess *webtransport.Session, target consolev1.WtWatchTarget, path, projectID, runID, host string) *webtransport.Stream {
	t.Helper()
	req, err := protojson.Marshal(&consolev1.WtWatchRequest{
		Target: target, Path: &path, ProjectId: &projectID, RunId: &runID, Host: &host,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The server ignores the request frame's type tag (kind + position
	// discriminate); optionals stay absent like the REST query strings.
	return wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_WATCH, "", "watch", req)
}

func TestWTWatchFsDeliversChange(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	dir := t.TempDir()
	str := wtOpenWatch(t, sess, consolev1.WtWatchTarget_WT_WATCH_TARGET_FS, dir, "", "", "")
	time.Sleep(time.Second) // let the server subscribe before the change lands
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		_ = str.SetReadDeadline(deadline)
		frame, err := wt.DecodeFrame(str)
		if err != nil {
			t.Fatalf("fsChange frame: %v", err)
		}
		if frame.GetType() != "fsChange" {
			continue
		}
		var body struct {
			ProjectPath string `json:"projectPath"`
		}
		if err := json.Unmarshal(frame.GetPayload(), &body); err != nil {
			t.Fatal(err)
		}
		if body.ProjectPath == dir {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("fsChange for %s never arrived: %s", dir, frame.GetPayload())
		}
	}
}

func TestWTWatchGitSnapshot(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	// The snapshot on subscribe is deterministic: no fs event needed.
	str := wtOpenWatch(t, sess, consolev1.WtWatchTarget_WT_WATCH_TARGET_GIT_STATUS, t.TempDir(), "", "", "")
	_ = str.SetReadDeadline(time.Now().Add(10 * time.Second))
	frame, err := wt.DecodeFrame(str)
	if err != nil {
		t.Fatalf("gitStatus frame: %v", err)
	}
	if frame.GetType() != "gitStatus" {
		t.Fatalf("first frame must be gitStatus, got %q", frame.GetType())
	}
	var body struct {
		Branch *string `json:"branch"`
		Clean  *bool   `json:"clean"`
	}
	if err := json.Unmarshal(frame.GetPayload(), &body); err != nil {
		t.Fatalf("gitStatus payload: %v", err)
	}
}

func TestWTWatchPortsSeesForward(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	str := wtOpenWatch(t, sess, consolev1.WtWatchTarget_WT_WATCH_TARGET_PORTS, "", "", "localhost", "")
	// Forward after subscribing: either the initial snapshot or the update
	// carries the port, so there is no subscribe race by construction.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if _, err := h.ports.Forward(port, ""); err != nil {
		t.Fatalf("forward: %v", err)
	}
	want := fmt.Sprint(port)
	deadline := time.Now().Add(15 * time.Second)
	for {
		_ = str.SetReadDeadline(deadline)
		frame, err := wt.DecodeFrame(str)
		if err != nil {
			t.Fatalf("ports frame: %v", err)
		}
		if frame.GetType() != "ports" {
			continue
		}
		if strings.Contains(string(frame.GetPayload()), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ports snapshot with %d never arrived: %s", port, frame.GetPayload())
		}
	}
}

func TestWTWatchNotifications(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	str := wtOpenWatch(t, sess, consolev1.WtWatchTarget_WT_WATCH_TARGET_NOTIFICATIONS, "", "", "", "")
	// Retry pushes until the server's subscription lands; Push is
	// non-blocking, so an early push may predate it.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		h.notifications.Push(&consolev1.NotificationEvent{Type: "notification", Kind: "test", Title: "hi"})
		_ = str.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		frame, err := wt.DecodeFrame(str)
		if err != nil {
			continue
		}
		if frame.GetType() != "notification" {
			continue
		}
		var body struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(frame.GetPayload(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Kind == "test" {
			return
		}
	}
	t.Fatal("notification frame never arrived")
}

func wtScriptProject(t *testing.T, h wtHarness, command string) (projectID, scriptID string) {
	t.Helper()
	root := t.TempDir()
	proj, err := h.projects.Create(projectssvc.CreateProjectOptions{Name: "P", Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	content := "[scripts]\n\n[scripts.echo]\nlabel = \"Echo\"\ncommand = \"" + command + "\"\n"
	if err := os.WriteFile(filepath.Join(root, "console.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj.ID, "echo"
}

func TestWTWatchScriptRunStreams(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	projectID, scriptID := wtScriptProject(t, h, "sleep 2; echo wt-script-marker")
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	if _, err := h.scripts.Run(projectID, scriptID); err != nil {
		t.Fatalf("run script: %v", err)
	}
	// Re-list to grab the running run's id through the public surface.
	str := wtOpenWatch(t, sess, consolev1.WtWatchTarget_WT_WATCH_TARGET_SCRIPT_RUN, "", projectID, currentScriptRunID(t, h, projectID), "")
	frames := wtReadFrames(t, str, 20*time.Second)
	types := frameTypes(frames)
	sawOutput, sawExit := false, false
	var output bytes.Buffer
	for _, f := range frames {
		if f.GetType() == "output" {
			sawOutput = true
			output.Write(f.GetPayload())
		}
		if f.GetType() == "exit" {
			sawExit = true
		}
	}
	if !sawOutput || !strings.Contains(output.String(), "wt-script-marker") {
		t.Fatalf("script output missing marker, got %v", types)
	}
	if !sawExit {
		t.Fatalf("script stream must end with an exit frame, got %v", types)
	}
}

func currentScriptRunID(t *testing.T, h wtHarness, projectID string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range h.scripts.ListRuns(projectID) {
			if r.Status == "running" {
				return r.RunID
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no running script run appeared")
	return ""
}

func TestWTWatchRejectsBadRequests(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	// Missing path.
	str := wtOpenWatch(t, sess, consolev1.WtWatchTarget_WT_WATCH_TARGET_FS, "", "", "", "")
	frames := wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("missing path must answer one error frame, got %v", frameTypes(frames))
	}
	// Unknown target enum (Go-constructed: protojson accepts the numeric).
	rawTarget, err := protojson.Marshal(&consolev1.WtWatchRequest{Target: 99})
	if err != nil {
		t.Fatal(err)
	}
	str = wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_WATCH, "", "watch", rawTarget)
	frames = wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("unknown target must answer one error frame, got %v", frameTypes(frames))
	}
	// Missing script run.
	str = wtOpenWatch(t, sess, consolev1.WtWatchTarget_WT_WATCH_TARGET_SCRIPT_RUN, "", "nope", "nope", "")
	frames = wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("missing run must answer one error frame, got %v", frameTypes(frames))
	}
}

func TestWTTunnelEchoesRawBytes(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	// Local TCP echo target.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}
	str := wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_PORT_TUNNEL, "", "tunnel", req)
	payload := []byte("hello-tunnel-raw-bytes")
	if _, err := str.Write(payload); err != nil {
		t.Fatalf("tunnel write: %v", err)
	}
	_ = str.SetReadDeadline(time.Now().Add(10 * time.Second))
	echo := make([]byte, len(payload))
	if _, err := io.ReadFull(str, echo); err != nil {
		t.Fatalf("tunnel echo: %v", err)
	}
	if !bytes.Equal(echo, payload) {
		t.Fatalf("tunnel corrupted bytes: %q", echo)
	}
}

func TestWTTunnelRejectsBadPorts(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	// Privileged port.
	req, err := protojson.Marshal(&consolev1.WtTunnelRequest{Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	str := wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_PORT_TUNNEL, "", "tunnel", req)
	frames := wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("privileged port must answer one error frame, got %v", frameTypes(frames))
	}
	// Closed port (nothing listening).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := uint32(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()
	req, err = protojson.Marshal(&consolev1.WtTunnelRequest{Port: closed})
	if err != nil {
		t.Fatal(err)
	}
	str = wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_PORT_TUNNEL, "", "tunnel", req)
	frames = wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("refused port must answer one error frame, got %v", frameTypes(frames))
	}
}

func TestWTDeviceStreamRejectsBadRequests(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	h := startWTTestServer(t, run.NewService(sessions))
	sess, _ := wtDial(t, h.addr, wtMint(t, h, "device-1"))
	if sess == nil {
		t.Fatal("dial failed")
	}
	// Empty device id (no simulator touched).
	req, err := protojson.Marshal(&consolev1.WtDeviceStreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	str := wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_DEVICE_STREAM, "", "device", req)
	frames := wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("empty device id must answer one error frame, got %v", frameTypes(frames))
	}
	// Unparseable request.
	str = wtOpenStream(t, sess, consolev1.WtStreamKind_WT_STREAM_KIND_DEVICE_STREAM, "", "device", []byte("not-proto"))
	frames = wtReadFrames(t, str, 10*time.Second)
	if len(frames) != 1 || frames[0].GetType() != "error" {
		t.Fatalf("bad request must answer one error frame, got %v", frameTypes(frames))
	}
}
