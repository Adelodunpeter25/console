/**
 * Types for system-prompt construction and context discovery.
 * Shapes loosely mirror oh-my-pi coding-agent discovery capabilities.
 */

/** Whether a discovered item came from user home config or the project tree. */
export type ConfigLevel = "user" | "project";

/** Metadata describing where a discovered item was loaded from. */
export interface SourceMeta {
  /** Logical provider id, e.g. "agents-md", "rules", "skills". */
  provider: string;
  /** Absolute path to the source file. */
  path: string;
  level: ConfigLevel;
}

/**
 * A context file that provides persistent instructions (AGENTS.md, CLAUDE.md, etc.).
 * Injected into the system prompt under repo-rules.
 */
export interface ContextFile {
  path: string;
  content: string;
  level: ConfigLevel;
  /** Distance from cwd (0 = in cwd, 1 = parent, …). Only for project files. */
  depth?: number;
  source: SourceMeta;
}

/**
 * Custom SYSTEM.md override that replaces or customizes the base identity block.
 * Project-level wins over user-level.
 */
export interface SystemPromptFile {
  path: string;
  content: string;
  level: ConfigLevel;
  source: SourceMeta;
}

/** Parsed frontmatter from a rule file. */
export interface RuleFrontmatter {
  description?: string;
  globs?: string[];
  alwaysApply?: boolean;
  [key: string]: unknown;
}

/**
 * A project/user rule. alwaysApply rules have full content injected into the prompt;
 * others appear as a short inventory for the model to request when relevant.
 */
export interface Rule {
  name: string;
  path: string;
  /** Body after frontmatter stripped. */
  content: string;
  description?: string;
  globs?: string[];
  alwaysApply?: boolean;
  level: ConfigLevel;
  source: SourceMeta;
}

/** Parsed frontmatter from a skill file. */
export interface SkillFrontmatter {
  name?: string;
  description?: string;
  globs?: string[];
  alwaysApply?: boolean;
  /** When true, omit from the system-prompt skill list (still loadable by name later). */
  hide?: boolean;
  /** Agent Skills standard alias for hide. */
  disableModelInvocation?: boolean;
  [key: string]: unknown;
}

/**
 * A skill: specialized knowledge / workflow the model can follow.
 * Listed by name+description in the prompt; full content is not inlined by default.
 */
export interface Skill {
  name: string;
  path: string;
  content: string;
  description?: string;
  frontmatter?: SkillFrontmatter;
  hide?: boolean;
  level: ConfigLevel;
  source: SourceMeta;
}

/**
 * A user-defined slash command loaded from `.agent/commands/*.md`.
 * Execution is Phase 6; discovery only for now.
 */
export interface SlashCommandFile {
  name: string;
  path: string;
  content: string;
  description?: string;
  level: ConfigLevel;
  source: SourceMeta;
}

/** Top-level workspace tree for the system prompt. */
export interface WorkspaceTree {
  rootPath: string;
  rendered: string;
  truncated: boolean;
  totalLines: number;
}

/** Host / environment lines shown in the workstation block. */
export interface EnvironmentInfo {
  date: string;
  cwd: string;
  os: string;
  arch: string;
  node?: string;
  gitBranch?: string;
  model?: string;
}

/** Aggregate of everything discovery found for one build. */
export interface DiscoveredContext {
  contextFiles: ContextFile[];
  systemPromptFile: SystemPromptFile | null;
  rules: Rule[];
  skills: Skill[];
  commands: SlashCommandFile[];
  workspaceTree: WorkspaceTree;
  environment: EnvironmentInfo;
}

import type { ApprovalMode } from "./tool.js";

/** Options for building the final system prompt string. */
export interface BuildSystemPromptOptions {
  /** Working directory to discover from. Default: process.cwd(). */
  cwd?: string;
  /** Home directory for user-level config. Default: os.homedir(). */
  home?: string;
  /** Stop walking ancestors at this path (e.g. repo root). */
  stopAt?: string;
  /** Model id for the workstation block. */
  model?: string;
  /** Tool names listed in the inventory. */
  toolNames?: string[];
  /** Active security approval mode ("always-ask" | "accept-edits" | "plan-mode" | "full-access"). */
  approvalMode?: ApprovalMode;
  /** Replace the default identity block entirely. */
  customPrompt?: string;
  /** Text appended after the assembled prompt. */
  appendPrompt?: string;
  /** Include a workspace tree summary. Default: true. */
  includeWorkspaceTree?: boolean;
  /** Skip filesystem discovery and use preloaded context. */
  preloaded?: Partial<DiscoveredContext>;
  /** Core identity override (used when no SYSTEM.md / customPrompt). */
  identity?: string;
}

/** Result of {@link buildSystemPrompt}. */
export interface BuildSystemPromptResult {
  /** Final single string suitable for Agent.systemPrompt / StreamFn. */
  systemPrompt: string;
  /** Named sections that were joined (useful for debugging). */
  sections: Array<{ name: string; content: string }>;
  /** Raw discovery payload used to build the prompt. */
  context: DiscoveredContext;
}
