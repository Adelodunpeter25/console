package tests

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestPortsTool_Unforward(t *testing.T) {
	pid := "proj_1"
	provider := &mockPortsProvider{
		ports: []types.ClientPort{
			{Port: 3000, URL: "http://localhost:45000", ProjectID: &pid},
			{Port: 5173, URL: "http://localhost:45001", ProjectID: &pid},
		},
	}

	tool := tools.NewPortsTool("proj_1", provider)

	// Valid unforward action
	args := helpers.MustJSONRaw(t, map[string]any{"action": "unforward", "port": 3000})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unforward: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "Port forward for port 3000 stopped successfully.") {
		t.Fatalf("unexpected unforward output: %s", text)
	}

	// Unforward non-existent port
	args = helpers.MustJSONRaw(t, map[string]any{"action": "unforward", "port": 9999})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "not currently forwarded") {
		t.Fatalf("expected not forwarded error, got: %v", err)
	}

	// Stop alias action
	args = helpers.MustJSONRaw(t, map[string]any{"action": "stop", "port": 5173})
	out, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("stop alias: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "Port forward for port 5173 stopped successfully.") {
		t.Fatalf("unexpected stop output: %s", text)
	}

	// Invalid port
	args = helpers.MustJSONRaw(t, map[string]any{"action": "unforward", "port": 0})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "greater than 0") {
		t.Fatalf("expected port > 0 error, got: %v", err)
	}
}

func TestPortsTool_ListWithListening(t *testing.T) {
	pid := "proj_1"
	provider := &mockPortsProvider{
		ports: []types.ClientPort{
			{Port: 3000, URL: "http://localhost:45000", ProjectID: &pid},
		},
		listening: []int{3000, 8080, 5173},
	}

	tool := tools.NewPortsTool("proj_1", provider)

	// List shows both active forward and unforwarded listening ports
	args := helpers.MustJSONRaw(t, map[string]any{"action": "list"})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "3000") || !strings.Contains(text, "45000") {
		t.Fatalf("expected forwarded port in list: %s", text)
	}
	if !strings.Contains(text, "Detected local listening ports") || !strings.Contains(text, "8080") || !strings.Contains(text, "5173") {
		t.Fatalf("expected unforwarded listening ports in list: %s", text)
	}

	// Empty forwards but listening ports exist
	emptyForwardsProvider := &mockPortsProvider{
		ports:     []types.ClientPort{},
		listening: []int{4000, 9000},
	}
	emptyTool := tools.NewPortsTool("proj_1", emptyForwardsProvider)
	out, err = emptyTool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("empty list with listening: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "No active port forwards found.") {
		t.Fatalf("expected no active port forwards header: %s", text)
	}
	if !strings.Contains(text, "Detected local listening ports") || !strings.Contains(text, "4000") || !strings.Contains(text, "9000") {
		t.Fatalf("expected detected listening ports: %s", text)
	}
}

func TestPortsTool_IntegrationLifecycle(t *testing.T) {
	registry := services.NewPortRegistry()
	defer registry.CloseAll()

	// Bind a local test listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	tool := tools.NewPortsTool("proj_test", registry)

	// Forward the port
	forwardArgs := helpers.MustJSONRaw(t, map[string]any{"action": "forward", "port": port})
	out, err := tool.Execute(context.Background(), forwardArgs)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, fmt.Sprintf("Port %d forwarded successfully", port)) {
		t.Fatalf("unexpected forward text: %s", text)
	}

	// List
	listArgs := helpers.MustJSONRaw(t, map[string]any{"action": "list"})
	out, err = tool.Execute(context.Background(), listArgs)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, strconv.Itoa(port)) {
		t.Fatalf("expected port %d in list: %s", port, text)
	}

	// Unforward
	unforwardArgs := helpers.MustJSONRaw(t, map[string]any{"action": "unforward", "port": port})
	out, err = tool.Execute(context.Background(), unforwardArgs)
	if err != nil {
		t.Fatalf("unforward: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, fmt.Sprintf("Port forward for port %d stopped successfully.", port)) {
		t.Fatalf("unexpected unforward text: %s", text)
	}

	// List after unforward
	out, err = tool.Execute(context.Background(), listArgs)
	if err != nil {
		t.Fatalf("list after unforward: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "No active port forwards found.") {
		t.Fatalf("expected no active port forwards after unforward: %s", text)
	}
}
