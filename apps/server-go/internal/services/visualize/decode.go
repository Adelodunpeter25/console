package visualize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// DecodeObject turns the tool's JSON object into the typed spec, rejecting
// unknown fields at every level.
func DecodeObject(input map[string]any) (*Spec, error) {
	if input == nil {
		return nil, tools.NewToolError("spec is required")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, tools.NewToolError("Invalid visual spec: %v", err)
	}
	var spec Spec
	if err := decodeStrict(raw, &spec); err != nil {
		return nil, tools.NewToolError("Invalid visual spec: %v", err)
	}
	return &spec, nil
}

// decodeStrict is the shared decoder for the root and each block payload.
func decodeStrict(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}
