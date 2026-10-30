// writePlan: the one file write plan mode allows. Plan mode denies every other
// write and exec tool, so this is how a read-only run leaves its plan behind.
//
// It is deliberately narrow, because it writes without an approval prompt:
//   - it writes only into <cwd>/.console/plans/, never to a path the model picks;
//   - the model supplies a short name, and the tool builds the filename itself;
//   - the folder carries its own .gitignore ("*"), so plans never reach git and
//     no tracked file is edited to make that so;
//   - the destination must not escape through a symlink.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
)

// PlanDir is where plans live, relative to the session's working directory.
const PlanDir = ".console/plans"

var planNameRe = regexp.MustCompile(`[^a-z0-9]+`)

type writePlanInput struct {
	Name    string `json:"name" jsonschema:"description=Short name for the plan, such as 'auth-refactor'. Letters, digits and dashes only; the file is saved as .console/plans/<name>.md."`
	Content string `json:"content" jsonschema:"description=The plan, in Markdown. Replaces an existing plan with the same name."`
}

// PlanSafe marks a tool that plan mode still allows although it is not a
// read-only tool. Only tools that are confined enough to be safe without an
// approval prompt may implement it.
type PlanSafe interface {
	PlanSafe() bool
}

type planTool struct {
	Tool
}

func (planTool) PlanSafe() bool { return true }

// sandboxedBash marks the plan-mode bash tool as safe to run in plan mode: it
// runs every command under the read-only sandbox, so the executor lets it
// through instead of denying its exec tier.
type sandboxedBash struct {
	Tool
}

func (sandboxedBash) PlanSafe() bool { return true }

// NewPlanModeBashTool is bash for plan mode. It is allowed to run because it
// runs commands in the read-only sandbox where the machine has one. Where it
// has none it runs commands normally, which is a deliberate choice by the
// owner, not an oversight.
func NewPlanModeBashTool(jobs *terminal.BashJobManager, ownerSessionID string, parentIDs ...string) Tool {
	return sandboxedBash{Tool: NewSandboxedBashTool(SandboxReadOnly, jobs, ownerSessionID, parentIDs...)}
}

// NewWritePlanTool builds writePlan, bound to one session's working directory.
// An empty cwd reports the tool unavailable rather than writing somewhere
// unexpected.
func NewWritePlanTool(cwd string) Tool {
	inner := NewTool("writePlan",
		"Save a plan as a Markdown file in .console/plans/. This is the only way to write a file in plan mode; use it when the user asks for a plan file. Replaces a plan with the same name.",
		TierWrite,
		func(ctx context.Context, in writePlanInput) (any, error) {
			path, err := SavePlan(cwd, in.Name, in.Content)
			if err != nil {
				return nil, err
			}
			return textResult(fmt.Sprintf("Plan saved to %s", path)), nil
		})
	return planTool{Tool: inner}
}

// PlanFileName turns a model-supplied name into a safe filename. It never
// returns a name containing a separator or a dot segment.
func PlanFileName(name string) (string, error) {
	slug := strings.Trim(planNameRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if slug == "" {
		return "", NewToolError("name must contain letters or digits, for example 'auth-refactor'")
	}
	if len(slug) > 80 {
		slug = strings.TrimRight(slug[:80], "-")
	}
	return slug + ".md", nil
}

// SavePlan writes a plan into cwd's plan folder and returns the path relative
// to cwd. It creates the folder and its .gitignore on first use.
func SavePlan(cwd, name, content string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return "", NewToolError("writePlan is unavailable: this session has no working directory")
	}
	if strings.TrimSpace(content) == "" {
		return "", NewToolError("content is empty; write the plan text")
	}
	file, err := PlanFileName(name)
	if err != nil {
		return "", err
	}

	root, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", NewToolError("working directory is not accessible: %v", err)
	}
	// Create the folder one level at a time and check each level is still inside
	// the root before going further. A single MkdirAll would follow a planted
	// .console -> elsewhere symlink and create directories at its target before
	// any check could run.
	realDir := root
	for _, part := range strings.Split(PlanDir, "/") {
		next := filepath.Join(realDir, part)
		info, statErr := os.Lstat(next)
		switch {
		case statErr == nil && info.Mode()&os.ModeSymlink != 0:
			return "", NewToolError("%s is a symlink; refusing to write through it", filepath.ToSlash(strings.TrimPrefix(next, root+string(filepath.Separator))))
		case statErr == nil && !info.IsDir():
			return "", NewToolError("%s exists and is not a directory", part)
		case statErr != nil && os.IsNotExist(statErr):
			if err := os.Mkdir(next, 0o755); err != nil {
				return "", NewToolError("could not create %s: %v", PlanDir, err)
			}
		case statErr != nil:
			return "", NewToolError("could not inspect %s: %v", PlanDir, statErr)
		}
		realDir = next
	}
	if !pathInside(root, realDir) {
		return "", NewToolError("%s resolves outside the working directory; refusing to write", PlanDir)
	}
	// The folder ignores itself, so no tracked file is touched.
	ignore := filepath.Join(realDir, ".gitignore")
	if _, err := os.Stat(ignore); os.IsNotExist(err) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return "", NewToolError("could not write %s: %v", ignore, err)
		}
	}

	target := filepath.Join(realDir, file)
	// A symlinked plan file would write through to its target.
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", NewToolError("%s is a symlink; refusing to write through it", file)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return "", NewToolError("could not write plan: %v", err)
	}
	return PlanDir + "/" + file, nil
}

func pathInside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
