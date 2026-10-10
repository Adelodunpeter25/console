// Path guard: confines write-capable tool calls to the session's project
// root, and normalizes the read roots that glob/grep would otherwise resolve
// against the server's launch directory.
//
// Reads are deliberately NOT confined. Agents legitimately read dependency
// caches, toolchain headers and reference repos; the hazard addressed here is
// writing outside the project, not reading it.
//
// bash's cwd is not confined either. The command string is opaque, so
// `cd ../other-project && ...` walks straight past a cwd check. Confining it
// would add breakage without adding protection.
package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Guard confines write-capable tool calls to one project root. A nil *Guard
// is a no-op, so callers that have no session root can pass nil.
type Guard struct {
	root string
}

// NewGuard builds a guard for one session root. It returns nil when root is
// empty or cannot be resolved, which disables confinement for that run rather
// than failing the run.
func NewGuard(root string) *Guard {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	// Resolve the root once so a symlinked checkout (/tmp on macOS, a linked
	// worktree) compares equal to the paths the model sends back.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return &Guard{root: filepath.Clean(abs)}
}

// Root is the absolute project root every write is checked against.
func (g *Guard) Root() string {
	if g == nil {
		return ""
	}
	return g.root
}

// Check validates a call's paths and returns the arguments to execute with.
// On rejection it returns a nil payload and an error naming the root.
func (g *Guard) Check(call tools.ToolCall) (json.RawMessage, error) {
	if g == nil || g.root == "" || len(call.Arguments) == 0 {
		return call.Arguments, nil
	}
	switch call.Name {
	case "write_file", "editFile":
		return g.checkSingleWrite(call)
	case "batchWrite":
		return g.checkBatchWrite(call)
	case "glob", "grep":
		return g.withDefaultRoot(call)
	}
	return call.Arguments, nil
}

// checkSingleWrite covers write_file and editFile, whose sole path lives under
// "path".
func (g *Guard) checkSingleWrite(call tools.ToolCall) (json.RawMessage, error) {
	fields, ok := decodeFields(call.Arguments)
	if !ok {
		return call.Arguments, nil // let the tool report its own decode error
	}
	raw, ok := fields["path"]
	if !ok {
		return call.Arguments, nil
	}
	var path string
	if err := json.Unmarshal(raw, &path); err != nil || path == "" {
		return call.Arguments, nil
	}
	resolved, err := g.resolveWrite(path)
	if err != nil {
		return nil, err
	}
	fields["path"] = mustMarshal(resolved)
	return json.Marshal(fields)
}

// checkBatchWrite is all-or-nothing: every file is resolved before any is
// returned, so a batch with one bad path writes nothing at all.
func (g *Guard) checkBatchWrite(call tools.ToolCall) (json.RawMessage, error) {
	fields, ok := decodeFields(call.Arguments)
	if !ok {
		return call.Arguments, nil
	}
	raw, ok := fields["files"]
	if !ok {
		return call.Arguments, nil
	}
	var files []json.RawMessage
	if err := json.Unmarshal(raw, &files); err != nil {
		return call.Arguments, nil
	}
	for i, file := range files {
		fileFields, ok := decodeFields(file)
		if !ok {
			continue
		}
		pathRaw, ok := fileFields["path"]
		if !ok {
			continue
		}
		var path string
		if err := json.Unmarshal(pathRaw, &path); err != nil || path == "" {
			continue
		}
		resolved, err := g.resolveWrite(path)
		if err != nil {
			return nil, fmt.Errorf("batchWrite rejected: file %d of %d. %v No files were written.", i+1, len(files), err)
		}
		fileFields["path"] = mustMarshal(resolved)
		encoded, err := json.Marshal(fileFields)
		if err != nil {
			return call.Arguments, nil
		}
		files[i] = encoded
	}
	fields["files"] = mustMarshal(files)
	return json.Marshal(fields)
}

// withDefaultRoot fills glob/grep's optional "root" with the session root.
// Both document it as "default: current dir", which today means the server's
// launch directory — so an omitted root recurses from somewhere arbitrary.
// An explicit root is left exactly as the model asked.
func (g *Guard) withDefaultRoot(call tools.ToolCall) (json.RawMessage, error) {
	fields, ok := decodeFields(call.Arguments)
	if !ok {
		return call.Arguments, nil
	}
	if raw, present := fields["root"]; present {
		var root string
		if err := json.Unmarshal(raw, &root); err == nil && root != "" {
			return call.Arguments, nil
		}
	}
	fields["root"] = mustMarshal(g.root)
	return json.Marshal(fields)
}

// resolveWrite rejects a non-absolute path, resolves symlinks, then requires
// containment. Symlinks are resolved so neither a link planted inside the
// project nor a path swapped after the check can redirect the write.
func (g *Guard) resolveWrite(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("write path %q must be absolute; this session's project is %q, so re-send it as an absolute path inside that directory.", path, g.root)
	}
	resolved, err := resolveDeepest(path)
	if err != nil {
		return "", err
	}
	if !withinRoot(g.root, resolved) {
		return "", fmt.Errorf("write path %q resolves to %q, which is outside this session's project (%s). Writes are confined to the project directory.", path, resolved, g.root)
	}
	return resolved, nil
}

// resolveDeepest resolves symlinks on the deepest existing ancestor of p and
// re-appends the remaining segments, so a path whose parent directories do not
// exist yet is still checked against its real location.
func resolveDeepest(p string) (string, error) {
	rest := ""
	cur := p
	// Bounded walk; a path cannot have more components than this on any
	// filesystem we support, and the bound stops a pathological input.
	for range 256 {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve path %q.", p)
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
	return "", fmt.Errorf("cannot resolve path %q.", p)
}

// withinRoot reports whether target is root itself or lives under it.
func withinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// decodeFields splits a JSON object into its raw fields. It reports false for
// anything that is not an object, so malformed arguments fall through to the
// tool's own validation instead of being rewritten here.
func decodeFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	return fields, true
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}
