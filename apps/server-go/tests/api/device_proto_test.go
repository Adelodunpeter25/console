// Device migration coverage: the /api/devices payloads must match the shared
// golden fixtures. Platform/state stay open-vocabulary strings; diskFreeBytes
// is uint64 and therefore encodes as a protojson string.
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

func deviceFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "device", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func checkDeviceFixture(t *testing.T, name string, msg proto.Message) {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != deviceFixture(t, name) {
		t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), deviceFixture(t, name))
	}
}

func checkDeviceListFixture(t *testing.T, name string, items []proto.Message) {
	t.Helper()
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := protojson.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		elements = append(elements, json.RawMessage(compactJSON(t, raw)))
	}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != deviceFixture(t, name) {
		t.Fatalf("%s drifted:\n got %s\nwant %s", name, encoded, deviceFixture(t, name))
	}
}

// Absent model/os_version must stay absent — "unknown" and "empty" differ.
func TestDeviceListFixture(t *testing.T) {
	checkDeviceListFixture(t, "list.json", []proto.Message{
		&consolev1.DeviceDescriptor{
			Id: "Pixel_7", Name: "Pixel_7", Platform: "android",
			State: "shutdown", IsAvailable: true,
		},
		&consolev1.DeviceDescriptor{
			Id: "970344EE-28D6-413C-99F7-319A3AFF5564", Name: "iPad (A16)", Platform: "ios",
			State: "booted", OsVersion: proto.String("26.3"), IsAvailable: true,
		},
	})
}

// diskFreeBytes is uint64 (a real disk exceeds 4 GB), so protojson emits it as
// a string. That is a wire change from the old JSON number and the reason this
// fixture is spelled with quotes.
func TestDeviceDiagnosticsFixture(t *testing.T) {
	checkDeviceFixture(t, "diagnostics.json", &consolev1.DeviceDiagnostics{
		XcodeInstalled: true, SimctlAvailable: true, AndroidSdkFound: true,
		AdbAvailable: true, EmulatorAvailable: true,
		DiskFreeBytes: 111201239040, HasEnoughDiskSpace: true,
	})
}

// The stream meta frame both players parse by hand: type is always "meta".
func TestDeviceStreamMetaFixture(t *testing.T) {
	checkDeviceFixture(t, "stream_meta.json", &consolev1.DeviceStreamMeta{
		Type: "meta", Width: 1170, Height: 2532, Name: "iPad (A16)", Codec: "avc1.42E01E",
	})
}

// Every coordinate is optional; only the ones the action uses travel.
func TestDeviceActionFixture(t *testing.T) {
	checkDeviceFixture(t, "action.json", &consolev1.DeviceActionRequest{
		Action: "swipe", X: proto.Float64(100.5), Y: proto.Float64(200.25),
		EndX: proto.Float64(300), EndY: proto.Float64(400), DurationMs: proto.Uint32(250),
	})
}