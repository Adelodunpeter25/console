// SystemPromptBuilder — assemble the final system prompt from layered
// sources. Port of apps/server/agent/src/systemprompt/builder.ts.
//
// Order: identity/SYSTEM.md, approval-mode instructions, skills inventory +
// always-apply rules, tool inventory, slash commands, repo context files
// (AGENTS.md, …), workspace tree, workstation (date/cwd/OS/git/model).
package systemprompt

import (
	"fmt"
	"strings"
)

// ApprovalMode mirrors ApprovalMode by value (not by import, to avoid a
// tools -> systemprompt -> permissions -> tools cycle: readSkill lives in
// tools and needs systemprompt, permissions needs tools.ToolTier).
type ApprovalMode string

const (
	AlwaysAsk   ApprovalMode = "always-ask"
	AcceptEdits ApprovalMode = "accept-edits"
	PlanMode    ApprovalMode = "plan-mode"
	FullAccess  ApprovalMode = "full-access"
)

const defaultIdentity = `You are a coding agent.
Use the available tools to explore the codebase, edit files, run shell commands, and accomplish user tasks correctly and concisely.`

// DefaultToolNames lists the built-in tool inventory shown when the caller
// doesn't supply an explicit set.
var DefaultToolNames = []string{
	"read_file", "readSkill", "list_dir", "glob", "grep",
	"write_file", "editFile", "batchWrite", "bash", "webSearch", "webFetch",
	"todo", "subagent", "ask",
}

// DiscoveredContext is everything discovery found for one build.
type DiscoveredContext struct {
	ContextFiles     []ContextFile
	SystemPromptFile *SystemPromptFile
	Rules            []Rule
	Skills           []Skill
	Commands         []SlashCommand
	WorkspaceTree    WorkspaceTree
	Environment      EnvironmentInfo
}

// BuildOptions configures one discovery + prompt-assembly run. The
// workspace tree section is included by default; set SkipWorkspaceTree to
// omit it (matches the TS `includeWorkspaceTree: false` option).
type BuildOptions struct {
	Cwd               string
	Home              string
	StopAt            string
	Model             string
	ToolNames         []string
	ApprovalMode      ApprovalMode
	CustomPrompt      string
	AppendPrompt      string
	SkipWorkspaceTree bool
	Identity          string
}

// Result is the outcome of BuildSystemPrompt.
type Result struct {
	SystemPrompt string
	Sections     []Section
	Context      DiscoveredContext
}

// Section is one named, non-empty block of the assembled prompt.
type Section struct {
	Name    string
	Content string
}

func section(name, content string) *Section {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}
	return &Section{Name: name, Content: trimmed}
}

// DiscoverContext runs every discovery loader and assembles the aggregate
// context for one cwd/home/stopAt.
func DiscoverContext(opts BuildOptions) DiscoveredContext {
	roots := ResolveRoots(opts.Cwd, opts.Home, opts.StopAt)

	tree := WorkspaceTree{RootPath: roots.Cwd}
	if !opts.SkipWorkspaceTree {
		tree = BuildWorkspaceTree(roots.Cwd)
	}

	return DiscoveredContext{
		ContextFiles:     DiscoverContextFiles(roots),
		SystemPromptFile: DiscoverSystemPromptFile(roots),
		Rules:            DiscoverRules(roots),
		Skills:           DiscoverSkills(roots),
		Commands:         DiscoverCommands(roots),
		WorkspaceTree:    tree,
		Environment:      CollectEnvironmentInfo(roots.Cwd, opts.Model),
	}
}

func renderIdentity(opts BuildOptions, systemMd string) string {
	if strings.TrimSpace(opts.CustomPrompt) != "" {
		return strings.TrimSpace(opts.CustomPrompt)
	}
	if strings.TrimSpace(systemMd) != "" {
		return strings.TrimSpace(systemMd)
	}
	if strings.TrimSpace(opts.Identity) != "" {
		return strings.TrimSpace(opts.Identity)
	}
	return defaultIdentity
}

func renderSkills(skills []Skill) string {
	var lines []string
	for _, s := range skills {
		if s.Hide {
			continue
		}
		desc := strings.TrimSpace(s.Description)
		if desc == "" {
			desc = "(no description)"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", s.Name, desc))
	}
	if len(lines) == 0 {
		return ""
	}
	out := []string{
		"# Skills",
		"Skills are specialized knowledge. When one matches the current task, call the `readSkill` tool with the skill name to load its full instructions before proceeding.",
		"<skills>",
	}
	out = append(out, lines...)
	out = append(out, "</skills>")
	return strings.Join(out, "\n")
}

func renderAlwaysApplyRules(rules []Rule) string {
	var blocks []string
	for _, r := range rules {
		if !r.AlwaysApply || strings.TrimSpace(r.Content) == "" {
			continue
		}
		blocks = append(blocks, fmt.Sprintf("<rule name=%q path=%q>\n%s\n</rule>", r.Name, r.Path, strings.TrimSpace(r.Content)))
	}
	if len(blocks) == 0 {
		return ""
	}
	out := []string{"# Always-apply rules", "<generic-rules>"}
	out = append(out, blocks...)
	out = append(out, "</generic-rules>")
	return strings.Join(out, "\n")
}

func renderDomainRules(rules []Rule) string {
	var lines []string
	for _, r := range rules {
		if r.AlwaysApply {
			continue
		}
		globs := ""
		if len(r.Globs) > 0 {
			globs = fmt.Sprintf(" (%s)", strings.Join(r.Globs, ", "))
		}
		desc := strings.TrimSpace(r.Description)
		if desc == "" {
			desc = "see rule file"
		}
		lines = append(lines, fmt.Sprintf("- %s%s: %s", r.Name, globs, desc))
	}
	if len(lines) == 0 {
		return ""
	}
	out := []string{"# Domain rules (request when relevant)", "<domain-rules>"}
	out = append(out, lines...)
	out = append(out, "</domain-rules>")
	return strings.Join(out, "\n")
}

func renderTools(names []string) string {
	if len(names) == 0 {
		return ""
	}
	lines := []string{"# Tool inventory"}
	for _, n := range names {
		lines = append(lines, fmt.Sprintf("- `%s`", n))
	}
	return strings.Join(lines, "\n")
}

func renderContextFiles(files []ContextFile) string {
	if len(files) == 0 {
		return ""
	}
	lines := []string{"# Repo rules", "You MUST follow the context files below for all tasks:", "<repo-rules>"}
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("<file path=%q>\n%s\n</file>", f.Path, strings.TrimSpace(f.Content)))
	}
	lines = append(lines, "</repo-rules>",
		"Context files above are loaded automatically. Do not waste tools grepping for AGENTS.md / CLAUDE.md unless the user asks.")
	return strings.Join(lines, "\n")
}

func renderWorkspaceTree(tree WorkspaceTree, skip bool) string {
	if skip || strings.TrimSpace(tree.Rendered) == "" {
		return ""
	}
	return strings.Join([]string{
		"# Workspace tree",
		"<workspace-tree>",
		"Working directory layout (depth-limited):",
		tree.Rendered,
		"</workspace-tree>",
	}, "\n")
}

func renderWorkstation(env EnvironmentInfo, mode ApprovalMode) string {
	lines := []string{
		fmt.Sprintf("Date: %s", env.Date),
		fmt.Sprintf("CWD: %s", env.Cwd),
		fmt.Sprintf("OS: %s", env.OS),
		fmt.Sprintf("Arch: %s", env.Arch),
	}
	if env.GitBranch != "" {
		lines = append(lines, fmt.Sprintf("Git branch: %s", env.GitBranch))
	}
	if env.Model != "" {
		lines = append(lines, fmt.Sprintf("Model: %s", env.Model))
	}

	var critical []string
	if mode == PlanMode {
		critical = []string{
			"<critical>",
			"- Each response MUST advance the task.",
			"- Default to discussion and questions; do not write files or implement features.",
			"</critical>",
		}
	} else {
		critical = []string{
			"<critical>",
			"- Each response MUST advance the task.",
			"- Default to informed action; do not ask for confirmation when tools or repo context can answer.",
			"- Never search code/files via bash (grep, rg, find). Use the dedicated `grep` and `glob` tools — they run on an indexed engine and are faster",
			"</critical>",
		}
	}

	out := []string{"# Workstation", "<workstation>"}
	for _, l := range lines {
		out = append(out, "- "+l)
	}
	out = append(out, "</workstation>", fmt.Sprintf("Today is %s. Working directory is '%s'.", env.Date, env.Cwd), "")
	out = append(out, critical...)
	return strings.Join(out, "\n")
}

func renderCommands(commands []SlashCommand) string {
	lines := []string{fmt.Sprintf("- /%s: %s. %s", InitCommandName, InitCommandDescription, InitCommandDetails)}
	for _, c := range commands {
		desc := strings.TrimSpace(c.Description)
		if desc == "" {
			desc = "user-defined command"
		}
		lines = append(lines, fmt.Sprintf("- /%s: %s", c.Name, desc))
	}
	out := []string{"# Available slash commands (may be invoked by the user)"}
	out = append(out, lines...)
	return strings.Join(out, "\n")
}

func renderApprovalModeInstruction(mode ApprovalMode) string {
	if mode == "" {
		return ""
	}
	heading := "# Approval Mode Active"
	tag := "<approval-mode-instructions>"

	switch mode {
	case AlwaysAsk:
		return strings.Join([]string{
			heading, tag,
			"You are in Normal mode. Use tools freely, but request approval before any write, edit, delete, or command execution.",
			"Do not claim a restricted action was performed until the user approves it.",
			"If approval is denied, stop and report the denial rather than retrying silently.",
			"</approval-mode-instructions>",
		}, "\n")
	case AcceptEdits:
		return strings.Join([]string{
			heading, tag,
			"You are in Accept Edits mode. Read, write, and edit files directly without pausing for each edit.",
			"Still request approval before executing commands or performing higher-risk operations.",
			"Proceed efficiently through multi-file edits; do not stop to ask permission for routine file writes.",
			"</approval-mode-instructions>",
		}, "\n")
	case PlanMode:
		return strings.Join([]string{
			heading, tag,
			"Plan mode is ACTIVE. Read-only by default:",
			"- You may read files, search the codebase, and ask clarifying questions to shape the design.",
			"- You MUST NOT create, edit, or delete files; run build/install/migration/test commands; commit, push, or spawn implementation subagents.",
			"- Plans live in chat. Only write a plan file when the user explicitly asks.",
			"- If the user asks you to implement, tell them to switch agents. Do not silently start implementing.",
			"</approval-mode-instructions>",
		}, "\n")
	case FullAccess:
		return strings.Join([]string{
			heading, tag,
			"You are in Bypass Permissions mode. Execute all necessary tools directly without asking for approval.",
			"You remain responsible for avoiding unrelated or destructive work; freedom is not a license to be careless.",
			"Report every action you take accurately so the user can audit what happened.",
			"</approval-mode-instructions>",
		}, "\n")
	default:
		return ""
	}
}

// BuildSystemPrompt runs discovery then assembles the final prompt string.
func BuildSystemPrompt(opts BuildOptions) Result {
	if opts.ToolNames == nil {
		opts.ToolNames = DefaultToolNames
	}
	context := DiscoverContext(opts)
	identity := renderIdentity(opts, systemMdContent(context.SystemPromptFile))

	var parts []Section
	add := func(s *Section) {
		if s != nil {
			parts = append(parts, *s)
		}
	}
	add(section("identity", identity))
	add(section("approval-mode", renderApprovalModeInstruction(opts.ApprovalMode)))
	add(section("skills", renderSkills(context.Skills)))
	add(section("always-apply-rules", renderAlwaysApplyRules(context.Rules)))
	add(section("domain-rules", renderDomainRules(context.Rules)))
	add(section("tools", renderTools(opts.ToolNames)))
	add(section("commands", renderCommands(context.Commands)))
	add(section("repo-rules", renderContextFiles(context.ContextFiles)))
	add(section("workspace-tree", renderWorkspaceTree(context.WorkspaceTree, opts.SkipWorkspaceTree)))
	add(section("workstation", renderWorkstation(context.Environment, opts.ApprovalMode)))
	add(section("append", opts.AppendPrompt))

	joined := make([]string, len(parts))
	for i, p := range parts {
		joined[i] = p.Content
	}

	return Result{SystemPrompt: strings.Join(joined, "\n\n"), Sections: parts, Context: context}
}

func systemMdContent(f *SystemPromptFile) string {
	if f == nil {
		return ""
	}
	return f.Content
}
