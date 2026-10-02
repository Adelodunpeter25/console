// Device simulator service: iOS simulators + Android emulators via the
// sim-go SDK. Owns state mapping, action dispatch, and the live stream pump.
// The pump is transport-agnostic (StreamConn); routes adapt fasthttp's
// websocket to it.
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/sim-go/sdk"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

var (
	ErrDeviceInvalidPlatform = sdk.ErrInvalidPlatform
	ErrDeviceNotFound        = sdk.ErrDeviceNotFound
	ErrDeviceUnsupported     = errors.New("action not supported")
	ErrDeviceBadRequest      = errors.New("bad request")
)

const (
	deviceBootTimeout   = 5 * time.Minute
	DeviceStreamPongTTL = 60 * time.Second
	deviceStreamPing    = DeviceStreamPongTTL * 9 / 10
)

type DeviceService struct {
	client *sdk.Client
}

func NewDeviceService() *DeviceService {
	return &DeviceService{client: sdk.New()}
}

// Close tears down pooled helpers (idb companions, stream sessions).
func (s *DeviceService) Close() error { return s.client.Close() }

func deviceState(d sdk.Device) string {
	switch d.State {
	case "Booted", "device":
		return "booted"
	case "Booting", "offline":
		return "booting"
	}
	return "shutdown"
}

func toDescriptor(d sdk.Device) types.DeviceDescriptor {
	out := types.DeviceDescriptor{ID: d.ID, Name: d.Name, Platform: d.Platform, State: deviceState(d), IsAvailable: true}
	if d.OS != "" && d.OS != "?" {
		v := d.OS
		out.OSVersion = &v
	}
	return out
}

func (s *DeviceService) List(ctx context.Context) ([]types.DeviceDescriptor, error) {
	devs, err := s.client.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.DeviceDescriptor, 0, len(devs))
	for _, d := range devs {
		out = append(out, toDescriptor(d))
	}
	return out, nil
}

func (s *DeviceService) Diagnostics(ctx context.Context) types.DeviceDiagnostics {
	d := s.client.Doctor(ctx)
	out := types.DeviceDiagnostics{
		XcodeInstalled:     d.XcodeInstalled,
		SimctlAvailable:    d.SimctlAvailable,
		AndroidSDKFound:    d.ADBAvailable || d.EmulatorAvail,
		ADBAvailable:       d.ADBAvailable,
		EmulatorAvailable:  d.EmulatorAvail,
		DiskFreeBytes:      d.DiskFreeBytes,
		HasEnoughDiskSpace: d.HasEnoughDiskGB,
		Errors:             []string{},
	}
	// Detail is a summary even when healthy; only surface real gaps.
	if !d.XcodeInstalled || !d.SimctlAvailable {
		out.Errors = append(out.Errors, "Xcode / simctl not available: iOS simulators disabled")
	}
	if !d.ADBAvailable {
		out.Errors = append(out.Errors, "adb not found: Android devices disabled")
	} else if !d.EmulatorAvail {
		out.Errors = append(out.Errors, "Android emulator not found: AVDs cannot be booted")
	}
	if !d.HasEnoughDiskGB {
		out.Errors = append(out.Errors, "Less than 10 GB of free disk space")
	}
	return out
}

// Boot validates the platform, then boots in the background: Android waits
// for boot_completed (minutes), so callers poll List instead of blocking.
func (s *DeviceService) Boot(platform, id string) error {
	if _, err := s.client.Driver(platform); err != nil {
		return fmt.Errorf("%w: %v", ErrDeviceInvalidPlatform, err)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), deviceBootTimeout)
		defer cancel()
		if err := s.client.Boot(ctx, platform, id); err != nil {
			slog.Warn("device boot failed", "platform", platform, "id", id, "error", err)
		}
	}()
	return nil
}

func (s *DeviceService) Shutdown(ctx context.Context, platform, id string) error {
	return s.client.Shutdown(ctx, platform, id)
}

// ShutdownAll powers off every booted device on every platform.
func (s *DeviceService) ShutdownAll(ctx context.Context) error {
	devs, err := s.client.ListAll(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range devs {
		if deviceState(d) != "booted" {
			continue
		}
		if err := s.client.Shutdown(ctx, d.Platform, d.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Name, err))
		}
	}
	return errors.Join(errs...)
}

// OpenApp installs a package path (.apk/.app) or launches a bundle/package id.
func (s *DeviceService) OpenApp(ctx context.Context, platform, id, app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("%w: app is required", ErrDeviceBadRequest)
	}
	if strings.HasSuffix(app, ".apk") || strings.HasSuffix(app, ".app") {
		return "", s.client.Install(ctx, platform, id, app)
	}
	return s.client.Launch(ctx, platform, id, app)
}

func pixel(v *float64) int {
	if v == nil {
		return 0
	}
	return int(*v)
}

// Interact dispatches a one-shot REST action. Unknown actions are treated
// as hardware buttons (home, back, power, volume_up, ...).
func (s *DeviceService) Interact(ctx context.Context, platform, id string, a types.DeviceAction) error {
	switch a.Action {
	case "tap":
		return s.client.Tap(ctx, platform, id, pixel(a.X), pixel(a.Y))
	case "swipe":
		ms := 300
		if a.DurationMs != nil {
			ms = *a.DurationMs
		}
		return s.client.Swipe(ctx, platform, id, pixel(a.X), pixel(a.Y), pixel(a.EndX), pixel(a.EndY), ms)
	case "text", "type":
		return s.client.Type(ctx, platform, id, a.Text)
	case "key":
		return s.client.Key(ctx, platform, id, a.Key)
	case "appearance":
		return fmt.Errorf("%w: appearance toggle", ErrDeviceUnsupported)
	case "":
		return fmt.Errorf("%w: action is required", ErrDeviceBadRequest)
	}
	return s.client.Press(ctx, platform, id, strings.ReplaceAll(a.Action, "_", "-"))
}

// Screenshot returns the current screen as PNG bytes.
func (s *DeviceService) Screenshot(ctx context.Context, platform, id string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "console-shot-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "screen.png")
	if err := s.client.Screenshot(ctx, platform, id, out); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}

// OpenStream attaches a viewer to a booted device. Call before the
// websocket upgrade so failures can still be plain HTTP errors.
func (s *DeviceService) OpenStream(ctx context.Context, platform, id string) (*sdk.Stream, error) {
	return s.client.Stream(ctx, platform, id)
}

// StreamConn is the slice of a websocket the pump needs. Writes happen
// only from PumpStream's goroutine; ReadText is called from another.
type StreamConn interface {
	WriteText(data []byte) error
	WriteBinary(data []byte) error
	WritePing() error
	WriteClose() error
	// ReadText blocks for the next text frame; non-text frames are skipped
	// by the adapter. Any error ends the stream.
	ReadText() ([]byte, error)
}

// PumpStream serves one viewer until either side ends. Wire:
//
//	server -> viewer  text    DeviceStreamMeta
//	server -> viewer  binary  [tag] payload (description | key | delta)
//	viewer -> server  text    sdk.Input JSON
//
// plus a ping every ~54s. Closes st.
func PumpStream(conn StreamConn, st *sdk.Stream) {
	var once sync.Once
	detach := func() { once.Do(func() { _ = st.Close() }) }
	defer detach()

	go func() {
		defer detach() // ends Packets, which ends the writer
		for {
			data, err := conn.ReadText()
			if err != nil {
				return
			}
			var in sdk.Input
			if json.Unmarshal(data, &in) != nil {
				continue
			}
			// Synchronous keeps touch down/move/up ordered; a rejected
			// input never ends the session.
			_ = st.Input(in)
		}
	}()

	ping := time.NewTicker(deviceStreamPing)
	defer ping.Stop()
	for {
		select {
		case p, ok := <-st.Packets():
			if !ok {
				_ = conn.WriteClose()
				return
			}
			if err := writePacket(conn, p); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WritePing(); err != nil {
				return
			}
		}
	}
}

func writePacket(conn StreamConn, p sdk.Packet) error {
	var tag byte
	switch p.Kind {
	case sdk.PacketMeta:
		b, err := json.Marshal(types.DeviceStreamMeta{
			Type: "meta", Width: p.Meta.Width, Height: p.Meta.Height,
			Name: p.Meta.Name, Codec: p.Meta.Codec,
		})
		if err != nil {
			return err
		}
		return conn.WriteText(b)
	case sdk.PacketDescription:
		tag = types.DeviceFrameDescription
	case sdk.PacketKeyFrame:
		tag = types.DeviceFrameKey
	case sdk.PacketDelta:
		tag = types.DeviceFrameDelta
	default:
		return nil // unknown kinds are skipped, never fatal
	}
	// p.Data is shared between viewers: copy, never mutate.
	out := make([]byte, 1+len(p.Data))
	out[0] = tag
	copy(out[1:], p.Data)
	return conn.WriteBinary(out)
}
