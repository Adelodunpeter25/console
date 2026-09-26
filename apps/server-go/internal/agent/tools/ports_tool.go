// ports tool: query listening ports and forwarded URLs, or manually forward ports.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// PortsProvider is the interface satisfied by *services.PortRegistry.
type PortsProvider interface {
	List(host, projectID string) []types.ClientPort
	Forward(port int, projectID string) (types.ClientPort, error)
}

type portsInput struct {
	Action string `json:"action" jsonschema:"required,enum=list,enum=forward,description=Action: 'list' all forwards or 'forward' a specific port."`
	Port   int    `json:"port,omitempty" jsonschema:"description=Remote port number to forward (required for 'forward')."`
}

func NewPortsTool(projectID string, provider PortsProvider) Tool {
	return NewTool(
		"ports",
		"Inspect remote listening ports and their forwarded local URLs on the desktop, or forward a specific port.",
		TierRead,
		func(ctx context.Context, input portsInput) (any, error) {
			if provider == nil {
				return nil, NewToolError("Ports service is unavailable.")
			}

			switch input.Action {
			case "list":
				ports := provider.List("localhost", projectID)
				if len(ports) == 0 {
					return textResult("No active port forwards found."), nil
				}
				data, err := json.MarshalIndent(ports, "", "  ")
				if err != nil {
					return nil, NewToolError("Failed to format ports: %v", err)
				}
				return textResult(string(data)), nil

			case "forward":
				if input.Port <= 0 {
					return nil, NewToolError("Port number is required and must be greater than 0 for 'forward' action.")
				}
				port, err := provider.Forward(input.Port, projectID)
				if err != nil {
					return nil, NewToolError("Failed to forward port %d: %v", input.Port, err)
				}
				data, err := json.MarshalIndent(port, "", "  ")
				if err != nil {
					return nil, NewToolError("Failed to format forwarded port: %v", err)
				}
				return textResult(fmt.Sprintf("Port %d forwarded successfully:\n%s", input.Port, string(data))), nil

			default:
				return nil, NewToolError("Unknown action: %s. Expected 'list' or 'forward'.", input.Action)
			}
		},
	)
}

// Ports is the unbound default instance used by DefaultTools() (nil provider).
var Ports = NewPortsTool("", nil)
