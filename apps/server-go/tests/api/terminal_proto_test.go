// Terminal migration coverage: proto-built control frames must match the
// shared golden fixtures, and client text frames must decode through the
// same oneof path the socket read loop uses. Raw PTY bytes and the binary
// tag framing are untouched and untested here.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

func terminalFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "terminal", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestTerminalProtoMatchesFixtures(t *testing.T) {
	pid := int32(123)
	cols := int32(80)
	rows := int32(24)
	zero := int32(0)
	check := func(name string, msg proto.Message) {
		t.Helper()
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if compactJSON(t, raw) != terminalFixture(t, name) {
			t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), terminalFixture(t, name))
		}
	}

	check("spawned.json", &consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Spawned{
		Spawned: &consolev1.TerminalSpawned{
			Id: "t1", Pid: pid, Cwd: "/tmp", Shell: "/bin/zsh", Cols: cols, Rows: rows,
		},
	}})
	check("output.json", &consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Output{
		Output: &consolev1.TerminalOutput{Data: "hi\n"},
	}})
	check("exit.json", &consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Exit{
		Exit: &consolev1.TerminalExit{Code: &zero},
	}})
	check("error.json", &consolev1.TerminalServerMessage{Event: &consolev1.TerminalServerMessage_Error{
		Error: &consolev1.TerminalError{Message: "boom"},
	}})
	check("input.json", &consolev1.TerminalClientMessage{Event: &consolev1.TerminalClientMessage_Input{
		Input: &consolev1.TerminalInput{Data: "ls\n"},
	}})
	check("resize.json", &consolev1.TerminalClientMessage{Event: &consolev1.TerminalClientMessage_Resize{
		Resize: &consolev1.TerminalResize{Cols: 100, Rows: 30},
	}})
	check("kill.json", &consolev1.TerminalClientMessage{Event: &consolev1.TerminalClientMessage_Kill{
		Kill: &consolev1.TerminalKill{},
	}})

	// Client frames decode through the read-loop path, unknown fields tolerated.
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	var frame consolev1.TerminalClientMessage
	if err := unmarshal.Unmarshal([]byte(terminalFixture(t, "resize.json")), &frame); err != nil {
		t.Fatal(err)
	}
	resize, ok := frame.GetEvent().(*consolev1.TerminalClientMessage_Resize)
	if !ok || resize.Resize.GetCols() != 100 {
		t.Fatalf("decoded: %+v", &frame)
	}
	var serverFrame consolev1.TerminalServerMessage
	if err := unmarshal.Unmarshal([]byte(terminalFixture(t, "spawned.json")), &serverFrame); err != nil {
		t.Fatal(err)
	}
	spawned, ok := serverFrame.GetEvent().(*consolev1.TerminalServerMessage_Spawned)
	if !ok || spawned.Spawned.GetPid() != 123 {
		t.Fatalf("decoded: %+v", &serverFrame)
	}
}
