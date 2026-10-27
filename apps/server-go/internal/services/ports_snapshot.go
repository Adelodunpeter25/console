// Shared rendering for the ports list surface.
package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// FormatPortsSnapshot renders the ports list view: active forwards as JSON,
// then any listening ports that are not forwarded yet. Shared by the ports
// tool and the console kernel so both read identically.
func FormatPortsSnapshot(ports []types.ClientPort, listening []int) (string, error) {
	forwarded := make(map[int]bool, len(ports))
	for _, port := range ports {
		forwarded[port.Port] = true
	}
	var pending []int
	for _, port := range listening {
		if !forwarded[port] {
			pending = append(pending, port)
		}
	}
	if len(ports) == 0 {
		if len(pending) > 0 {
			return fmt.Sprintf("No active port forwards found.\n\nDetected local listening ports (available to forward):\n%s", formatPortNumbers(pending)), nil
		}
		return "No active port forwards found.", nil
	}
	data, err := json.MarshalIndent(ports, "", "  ")
	if err != nil {
		return "", err
	}
	result := string(data)
	if len(pending) > 0 {
		result = fmt.Sprintf("%s\n\nDetected local listening ports (available to forward):\n%s", result, formatPortNumbers(pending))
	}
	return result, nil
}

func formatPortNumbers(ports []int) string {
	lines := make([]string, len(ports))
	for i, port := range ports {
		lines[i] = fmt.Sprintf("- %d", port)
	}
	return strings.Join(lines, "\n")
}
