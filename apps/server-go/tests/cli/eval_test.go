// computer-use eval: the command fails honestly without a driver, and the
// output rendering never leaks raw base64 to a terminal.
package tests

import (
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/commands"
)

func TestRunEvalWithoutDriverExplainsItself(t *testing.T) {
	t.Setenv("CUA_DRIVER_DISABLED", "1")
	_, err := commands.RunEvalForTest("1+1", 0)
	if err == nil {
		t.Fatal("expected unavailability, got success")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("got %v", err)
	}
}

func TestRenderEvalLinesImageCarriesSizeNotBytes(t *testing.T) {
	// A megabyte of base64 must never reach stdout: the line carries the
	// decoded size instead.
	lines := commands.RenderEvalLinesForTest([]map[string]any{
		{"type": "text", "text": "hello"},
		{"type": "image", "mimeType": "image/png", "data": strings.Repeat("QUJD", 1000)},
	})
	if len(lines) != 2 || lines[0] != "hello" {
		t.Fatalf("text mishandled: %v", lines)
	}
	if strings.Contains(lines[1], "QUJD") {
		t.Fatalf("raw base64 leaked: %q...", lines[1][:60])
	}
	if !strings.Contains(lines[1], "image/png") || !strings.Contains(lines[1], "3000") {
		t.Errorf("size line wrong: %q", lines[1])
	}
}
