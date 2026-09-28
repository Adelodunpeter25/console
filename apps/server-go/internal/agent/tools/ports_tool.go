// ports tool: query listening ports and forwarded URLs, or manually forward/unforward ports.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// PortsProvider is the interface satisfied by *services.PortRegistry.
type PortsProvider interface {
	List(host, projectID string) []types.ClientPort
	Forward(port int, projectID string) (types.ClientPort, error)
	Unforward(port int, projectID string) bool
	DetectListening() []int
}

type portsInput struct {
	Action string `json:"action" jsonschema:"required,enum=list,enum=forward,enum=unforward,enum=stop,description=Action: 'list' all forwards, 'forward' a specific port, or 'unforward'/'stop' an active forward."`
	Port   int    `json:"port,omitempty" jsonschema:"description=Remote port number to forward or unforward (required for 'forward', 'unforward', and 'stop')."`
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
				listening := provider.DetectListening()

				forwardedMap := make(map[int]bool)
				for _, p := range ports {
					forwardedMap[p.Port] = true
				}
				var unforwardedListening []int
				for _, lp := range listening {
					if !forwardedMap[lp] {
						unforwardedListening = append(unforwardedListening, lp)
					}
				}

				if len(ports) == 0 {
					if len(unforwardedListening) > 0 {
						listeningStrs := make([]string, len(unforwardedListening))
						for i, p := range unforwardedListening {
							listeningStrs[i] = fmt.Sprintf("- %d", p)
						}
						return textResult(fmt.Sprintf("No active port forwards found.\n\nDetected local listening ports (available to forward):\n%s", strings.Join(listeningStrs, "\n"))), nil
					}
					return textResult("No active port forwards found."), nil
				}

				data, err := json.MarshalIndent(ports, "", "  ")
				if err != nil {
					return nil, NewToolError("Failed to format ports: %v", err)
				}
				result := string(data)
				if len(unforwardedListening) > 0 {
					listeningStrs := make([]string, len(unforwardedListening))
					for i, p := range unforwardedListening {
						listeningStrs[i] = fmt.Sprintf("- %d", p)
					}
					result = fmt.Sprintf("%s\n\nDetected local listening ports (available to forward):\n%s", result, strings.Join(listeningStrs, "\n"))
				}
				return textResult(result), nil

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

			case "unforward", "stop":
				if input.Port <= 0 {
					return nil, NewToolError("Port number is required and must be greater than 0 for '%s' action.", input.Action)
				}
				if !provider.Unforward(input.Port, projectID) {
					return nil, NewToolError("Port %d is not currently forwarded.", input.Port)
				}
				return textResult(fmt.Sprintf("Port forward for port %d stopped successfully.", input.Port)), nil

			default:
				return nil, NewToolError("Unknown action: %s. Expected 'list', 'forward', 'unforward', or 'stop'.", input.Action)
			}
		},
	)
}

// Ports is the unbound default instance used by DefaultTools() (nil provider).
var Ports = NewPortsTool("", nil)
