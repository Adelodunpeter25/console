/**
 * SystemPromptBuilder — assemble the final system prompt from layered sources.
 *
 * Order (mirrors oh-my-pi system-prompt.ts + project-prompt.md):
 *  1. Identity / SYSTEM.md / custom prompt
 *  2. Skills inventory + always-apply rules
 *  3. Tool inventory
 *  4. Repo context files (AGENTS.md, …)
 *  5. Workspace tree
 *  6. Workstation (date, cwd, OS, git branch, model)
 *  7. Append prompt
 */
import type {
  ApprovalMode,
  BuildSystemPromptOptions,
  BuildSystemPromptResult,
  DiscoveredContext,
  Rule,
  Skill,
} from "@/agent/src/types/index.js";
import { DEFAULT_IDENTITY, DEFAULT_TOOL_NAMES } from "./defaults.js";
import { discoverContextFiles } from "./discover-agents-md.js";
import { discoverCommands } from "./discover-commands.js";
import { discoverRules } from "./discover-rules.js";
import { discoverSkills } from "./discover-skills.js";
import { discoverSystemPromptFile } from "./discover-system-md.js";
import { collectEnvironmentInfo } from "./environment.js";
import { resolveRoots } from "./walk.js";
import { buildWorkspaceTree } from "./workspace-tree.js";

function section(
  name: string,
  content: string | undefined | null,
): { name: string; content: string } | null {
  const trimmed = content?.trim();
  if (!trimmed) return null;
  return { name, content: trimmed };
}

function joinSections(sections: Array<{ name: string; content: string }>): string {
  return sections.map((s) => s.content).join("\n\n");
}

function renderIdentity(options: BuildSystemPromptOptions, systemMd: string | null): string {
  if (options.customPrompt?.trim()) return options.customPrompt.trim();
  if (systemMd?.trim()) return systemMd.trim();
  if (options.identity?.trim()) return options.identity.trim();
  return DEFAULT_IDENTITY;
}

function renderSkills(skills: Skill[]): string {
  const visible = skills.filter((s) => !s.hide);
  if (visible.length === 0) return "";
  const lines = visible.map((s) => {
    const desc = s.description?.trim() || "(no description)";
    return `- ${s.name}: ${desc}`;
  });
  return [
    "# Skills",
    "Skills are specialized knowledge. When one matches the current task, call the `readSkill` tool with the skill name to load its full instructions before proceeding.",
    "<skills>",
    ...lines,
    "</skills>",
  ].join("\n");
}

function renderAlwaysApplyRules(rules: Rule[]): string {
  const always = rules.filter((r) => r.alwaysApply && r.content.trim());
  if (always.length === 0) return "";
  const blocks = always.map(
    (r) => `<rule name="${r.name}" path="${r.path}">\n${r.content.trim()}\n</rule>`,
  );
  return ["# Always-apply rules", "<generic-rules>", ...blocks, "</generic-rules>"].join("\n");
}

function renderDomainRules(rules: Rule[]): string {
  const domain = rules.filter((r) => !r.alwaysApply);
  if (domain.length === 0) return "";
  const lines = domain.map((r) => {
    const globs = r.globs?.length ? ` (${r.globs.join(", ")})` : "";
    const desc = r.description?.trim() || "see rule file";
    return `- ${r.name}${globs}: ${desc}`;
  });
  return [
    "# Domain rules (request when relevant)",
    "<domain-rules>",
    ...lines,
    "</domain-rules>",
  ].join("\n");
}

function renderTools(toolNames: string[]): string {
  if (toolNames.length === 0) return "";
  return ["# Tool inventory", ...toolNames.map((n) => `- \`${n}\``)].join("\n");
}

function renderContextFiles(files: DiscoveredContext["contextFiles"]): string {
  if (files.length === 0) return "";
  const blocks = files.map((f) => `<file path="${f.path}">\n${f.content.trim()}\n</file>`);
  return [
    "# Repo rules",
    "You MUST follow the context files below for all tasks:",
    "<repo-rules>",
    ...blocks,
    "</repo-rules>",
    "Context files above are loaded automatically. Do not waste tools grepping for AGENTS.md / CLAUDE.md unless the user asks.",
  ].join("\n");
}

function renderWorkspaceTree(tree: DiscoveredContext["workspaceTree"], enabled: boolean): string {
  if (!enabled || !tree.rendered.trim()) return "";
  return [
    "# Workspace tree",
    "<workspace-tree>",
    `Working directory layout (depth-limited):`,
    tree.rendered,
    "</workspace-tree>",
  ].join("\n");
}

function renderWorkstation(env: DiscoveredContext["environment"]): string {
  const lines = [`Date: ${env.date}`, `CWD: ${env.cwd}`, `OS: ${env.os}`, `Arch: ${env.arch}`];
  if (env.node) lines.push(`Node: ${env.node}`);
  if (env.gitBranch) lines.push(`Git branch: ${env.gitBranch}`);
  if (env.model) lines.push(`Model: ${env.model}`);

  return [
    "# Workstation",
    "<workstation>",
    ...lines.map((l) => `- ${l}`),
    "</workstation>",
    `Today is ${env.date}. Working directory is '${env.cwd}'.`,
    "",
    "<critical>",
    "- Each response MUST advance the task.",
    "- Default to informed action; do not ask for confirmation when tools or repo context can answer.",
    "- Verify significant behavioral changes before yielding when tools allow.",
    "- Never search code/files via bash (grep, rg, find). Use the dedicated `grep` and `glob` tools — they run on an indexed engine and are faster.",
    "</critical>",
  ].join("\n");
}

function renderCommands(commands: DiscoveredContext["commands"]): string {
  if (commands.length === 0) return "";
  const lines = commands.map((c) => {
    const desc = c.description?.trim() || "user-defined command";
    return `- /${c.name}: ${desc}`;
  });
  return ["# Available slash commands (user-defined; may be invoked by the user)", ...lines].join(
    "\n",
  );
}

/**
 * Run all discovery loaders and assemble a full {@link DiscoveredContext}.
 */
export async function discoverContext(
  options: BuildSystemPromptOptions = {},
): Promise<DiscoveredContext> {
  const roots = resolveRoots(options);
  const pre = options.preloaded ?? {};

  const [contextFiles, systemPromptFile, rules, skills, commands, workspaceTree, environment] =
    await Promise.all([
      pre.contextFiles ? Promise.resolve(pre.contextFiles) : discoverContextFiles(roots),
      pre.systemPromptFile !== undefined
        ? Promise.resolve(pre.systemPromptFile)
        : discoverSystemPromptFile(roots),
      pre.rules ? Promise.resolve(pre.rules) : discoverRules(roots),
      pre.skills ? Promise.resolve(pre.skills) : discoverSkills(roots),
      pre.commands ? Promise.resolve(pre.commands) : discoverCommands(roots),
      pre.workspaceTree
        ? Promise.resolve(pre.workspaceTree)
        : options.includeWorkspaceTree === false
          ? Promise.resolve({
              rootPath: roots.cwd,
              rendered: "",
              truncated: false,
              totalLines: 0,
            })
          : buildWorkspaceTree(roots.cwd),
      pre.environment
        ? Promise.resolve(pre.environment)
        : collectEnvironmentInfo({ cwd: roots.cwd, model: options.model }),
    ]);

  return {
    contextFiles,
    systemPromptFile,
    rules,
    skills,
    commands,
    workspaceTree,
    environment,
  };
}

function renderApprovalModeInstruction(mode?: ApprovalMode): string {
  if (!mode) return "";
  const heading = "# Approval Mode Active";
  const tag = "<approval-mode-instructions>";

  switch (mode) {
    case "always-ask":
      return [
        heading,
        tag,
        "You are in Normal mode. Use tools freely, but request approval before any write, edit, delete, or command execution.",
        "Do not claim a restricted action was performed until the user approves it.",
        "If approval is denied, stop and report the denial rather than retrying silently.",
        "</approval-mode-instructions>",
      ].join("\n");

    case "accept-edits":
      return [
        heading,
        tag,
        "You are in Accept Edits mode. Read, write, and edit files directly without pausing for each edit.",
        "Still request approval before executing commands or performing higher-risk operations.",
        "Proceed efficiently through multi-file edits; do not stop to ask permission for routine file writes.",
        "</approval-mode-instructions>",
      ].join("\n");

    case "plan-mode":
      return [
        heading,
        tag,
        "Plan mode is ACTIVE. You have FULL permissions to explore and act:",
        "- Freely explore the codebase, read files, run diagnostic commands, and use subagents.",
        "- You MAY write, edit, and execute commands as needed to verify your plan.",
        "- Formulate a detailed, step-by-step implementation plan artifact.",
        "- Present your proposed implementation plan for user review, then proceed to execution.",
        "</approval-mode-instructions>",
      ].join("\n");

    case "full-access":
      return [
        heading,
        tag,
        "You are in Bypass Permissions mode. Execute all necessary tools directly without asking for approval.",
        "You remain responsible for avoiding unrelated or destructive work; freedom is not a license to be careless.",
        "Report every action you take accurately so the user can audit what happened.",
        "</approval-mode-instructions>",
      ].join("\n");

    default:
      return "";
  }
}

/**
 * Build the final system prompt string from discovery + options.
 */
export async function buildSystemPrompt(
  options: BuildSystemPromptOptions = {},
): Promise<BuildSystemPromptResult> {
  const context = await discoverContext(options);
  const toolNames = options.toolNames ?? [...DEFAULT_TOOL_NAMES];
  const includeTree = options.includeWorkspaceTree !== false;

  const identity = renderIdentity(options, context.systemPromptFile?.content ?? null);

  const parts = [
    section("identity", identity),
    section("approval-mode", renderApprovalModeInstruction(options.approvalMode)),
    section("skills", renderSkills(context.skills)),
    section("always-apply-rules", renderAlwaysApplyRules(context.rules)),
    section("domain-rules", renderDomainRules(context.rules)),
    section("tools", renderTools(toolNames)),
    section("commands", renderCommands(context.commands)),
    section("repo-rules", renderContextFiles(context.contextFiles)),
    section("workspace-tree", renderWorkspaceTree(context.workspaceTree, includeTree)),
    section("workstation", renderWorkstation(context.environment)),
    section("append", options.appendPrompt),
  ].filter((s): s is { name: string; content: string } => s !== null);

  return {
    systemPrompt: joinSections(parts),
    sections: parts,
    context,
  };
}

/**
 * Convenience class for callers that prefer an instance API.
 */
export class SystemPromptBuilder {
  constructor(private readonly defaults: BuildSystemPromptOptions = {}) {}

  async build(overrides: BuildSystemPromptOptions = {}): Promise<BuildSystemPromptResult> {
    return buildSystemPrompt({ ...this.defaults, ...overrides });
  }

  async discover(overrides: BuildSystemPromptOptions = {}): Promise<DiscoveredContext> {
    return discoverContext({ ...this.defaults, ...overrides });
  }
}
