/**
 * Discover always-apply / inventory rules from `.agent/rules`, `.console/rules`, etc.
 * Inspired by oh-my-pi rule capability + cursor/windsurf providers.
 */
import * as path from "node:path";
import type { Rule } from "@/agent/src/types/system-prompt.js";
import { asBoolean, asStringArray, parseFrontmatter } from "./frontmatter.js";
import {
  createSourceMeta,
  getProjectConfigDirs,
  getUserConfigDirs,
  listMarkdownFiles,
  readTextFile,
  resolveRoots,
} from "./walk.js";

function ruleNameFromPath(filePath: string): string {
  return path.basename(filePath).replace(/\.(mdc|md|markdown)$/i, "");
}

async function loadRuleFile(filePath: string, level: "user" | "project"): Promise<Rule | null> {
  const raw = await readTextFile(filePath);
  if (raw === null || !raw.trim()) return null;

  const { frontmatter, body } = parseFrontmatter(raw);
  const name =
    typeof frontmatter.name === "string" && frontmatter.name.trim()
      ? frontmatter.name.trim()
      : ruleNameFromPath(filePath);

  return {
    name,
    path: path.resolve(filePath),
    content: body.trim(),
    description: typeof frontmatter.description === "string" ? frontmatter.description : undefined,
    globs: asStringArray(frontmatter.globs),
    alwaysApply: asBoolean(frontmatter.alwaysApply) ?? false,
    level,
    source: createSourceMeta("rules", filePath, level),
  };
}

async function loadRulesFromDir(dir: string, level: "user" | "project"): Promise<Rule[]> {
  const files = await listMarkdownFiles(path.join(dir, "rules"));
  const rules: Rule[] = [];
  for (const file of files) {
    const rule = await loadRuleFile(file, level);
    if (rule) rules.push(rule);
  }
  return rules;
}

/**
 * Load rules from user and project config dirs.
 * Project rules override user rules with the same name.
 */
export async function discoverRules(
  options: { cwd?: string; home?: string; stopAt?: string } = {},
): Promise<Rule[]> {
  const roots = resolveRoots(options);
  const byName = new Map<string, Rule>();

  // User first, then project (project overwrites)
  for (const userDir of await getUserConfigDirs(roots.home)) {
    for (const rule of await loadRulesFromDir(userDir, "user")) {
      byName.set(rule.name, rule);
    }
  }

  // Closer project dirs last so they win
  const projectDirs = await getProjectConfigDirs(roots.cwd, roots.stopAt);
  const ordered = [...projectDirs].sort((a, b) => b.depth - a.depth);
  for (const { dir } of ordered) {
    for (const rule of await loadRulesFromDir(dir, "project")) {
      byName.set(rule.name, rule);
    }
  }

  return Array.from(byName.values()).sort((a, b) => a.name.localeCompare(b.name));
}
