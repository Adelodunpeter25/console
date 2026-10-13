// Shared entity-to-proto converters for transports. The SSE routes and the
// WebTransport server render identical payloads from these, so client
// parsers are shared across transports. Pure field mappings — no I/O, no
// transport types.
package services

import (
	"fmt"
	"os"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func gitFileEntryToProto(f types.GitFileEntry) *consolev1.GitFileEntry {
	additions := uint32(f.Additions)
	deletions := uint32(f.Deletions)
	return &consolev1.GitFileEntry{
		Path: f.Path, Status: string(f.Status), Staged: f.Staged,
		Additions: &additions, Deletions: &deletions,
	}
}

// GitSummaryToProto converts a git status summary to the canonical wire type.
func GitSummaryToProto(s types.GitStatusSummary) *consolev1.GitStatusSummary {
	out := &consolev1.GitStatusSummary{Branch: s.Branch, Clean: s.Clean}
	for _, f := range s.Files {
		out.Files = append(out.Files, gitFileEntryToProto(f))
	}
	return out
}

// clientPortToProto converts a registry row to the canonical wire type.
func clientPortToProto(p types.ClientPort) *consolev1.ForwardedPort {
	out := &consolev1.ForwardedPort{Port: int32(p.Port), Url: p.URL}
	if p.ProjectID != nil {
		out.ProjectId = p.ProjectID
	}
	return out
}

// PortsToProto converts registry rows to the canonical wire type.
func PortsToProto(list []types.ClientPort) []*consolev1.ForwardedPort {
	out := make([]*consolev1.ForwardedPort, 0, len(list))
	for _, p := range list {
		out = append(out, clientPortToProto(p))
	}
	return out
}

// ScriptRunEventToProto converts a script run event to the canonical wire
// type plus its SSE event name (output/exit/status).
func ScriptRunEventToProto(e types.ScriptRunEvent) (*consolev1.ScriptRunEvent, string) {
	switch e.Type {
	case "output":
		return &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Output{
			Output: &consolev1.ScriptOutputEvent{Stream: e.Stream, Text: e.Text},
		}}, "output"
	case "exit":
		out := &consolev1.ScriptExitEvent{Status: string(e.Status)}
		if e.ExitCode != nil {
			out.ExitCode = func() *int32 { v := int32(*e.ExitCode); return &v }()
		}
		return &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Exit{Exit: out}}, "exit"
	default:
		return &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Status{
			Status: &consolev1.ScriptStatusEvent{Status: string(e.Status)},
		}}, "status"
	}
}

// ResolveTerminalSpawnParams validates and defaults terminal spawn params,
// shared by the WS and WT transports so both enforce identical bounds.
// Non-positive cols/rows select the 80x24 default; the binary flag selects
// tag-byte binary output.
func ResolveTerminalSpawnParams(cwd, shell, label string, cols, rows int, binary bool) (types.TerminalSpawnParams, error) {
	if len(cwd) > 4096 {
		return types.TerminalSpawnParams{}, fmt.Errorf("cwd too long")
	}
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			return types.TerminalSpawnParams{}, err
		}
	}
	if len(shell) > 1024 {
		return types.TerminalSpawnParams{}, fmt.Errorf("shell path too long")
	}
	if len(label) > 256 {
		return types.TerminalSpawnParams{}, fmt.Errorf("label too long")
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return types.TerminalSpawnParams{
		Cwd: cwd, Shell: shell,
		Cols:   min(max(cols, 1), 500),
		Rows:   min(max(rows, 1), 200),
		Label:  label,
		Binary: binary,
	}, nil
}
