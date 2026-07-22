/**
 * System prompt construction & context discovery.
 *
 * Modular layout (oh-my-pi inspired, simplified):
 *   walk / frontmatter  — shared helpers
 *   discover-*          — AGENTS.md, SYSTEM.md, rules, skills, commands
 *   workspace-tree      — cwd tree for the prompt
 *   environment         — date, OS, git branch
 *   builder             — assemble final prompt
 */

export { buildSystemPrompt, discoverContext, SystemPromptBuilder } from "./builder.js";
export { DEFAULT_IDENTITY, DEFAULT_TOOL_NAMES } from "./defaults.js";
export { discoverContextFiles } from "./discover-agents-md.js";
export { discoverCommands } from "./discover-commands.js";
export { discoverRules } from "./discover-rules.js";
export { discoverSkills } from "./discover-skills.js";
export { discoverSystemPromptFile } from "./discover-system-md.js";
export { collectEnvironmentInfo } from "./environment.js";
export {
  asBoolean,
  asStringArray,
  parseFrontmatter,
  type ParsedFrontmatter,
} from "./frontmatter.js";
export {
  createSourceMeta,
  getAncestorDirs,
  getProjectConfigDirs,
  getUserConfigDirs,
  PROJECT_CONFIG_DIRS,
  resolveRoots,
  USER_CONFIG_DIRS,
} from "./walk.js";
export { buildWorkspaceTree } from "./workspace-tree.js";
