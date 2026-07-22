/**
 * Discover skills from `skills/` config dirs.
 * Supports:
 *   - skills/<name>.md
 *   - skills/<name>/SKILL.md  (Agent Skills convention)
 * Inspired by oh-my-pi skill capability + scanSkillsFromDir.
 */
import * as fs from "node:fs/promises";
import * as path from "node:path";
import type { Skill } from "../types/system-prompt.js";
import { asBoolean, parseFrontmatter } from "./frontmatter.js";
import {
  createSourceMeta,
  getProjectConfigDirs,
  getUserConfigDirs,
  isDirectory,
  listMarkdownFiles,
  readTextFile,
  resolveRoots,
} from "./walk.js";

function skillNameFromPath(filePath: string, frontmatterName?: string): string {
  if (frontmatterName?.trim()) return frontmatterName.trim();
  const base = path.basename(filePath);
  if (base.toLowerCase() === "skill.md") {
    return path.basename(path.dirname(filePath));
  }
  return base.replace(/\.(mdc|md|markdown)$/i, "");
}

async function loadSkillFile(filePath: string, level: "user" | "project"): Promise<Skill | null> {
  const raw = await readTextFile(filePath);
  if (raw === null || !raw.trim()) return null;

  const { frontmatter, body } = parseFrontmatter(raw);
  const name = skillNameFromPath(
    filePath,
    typeof frontmatter.name === "string" ? frontmatter.name : undefined,
  );
  const hide =
    asBoolean(frontmatter.hide) === true || asBoolean(frontmatter.disableModelInvocation) === true;

  return {
    name,
    path: path.resolve(filePath),
    content: body.trim(),
    description: typeof frontmatter.description === "string" ? frontmatter.description : undefined,
    frontmatter,
    hide,
    level,
    source: createSourceMeta("skills", filePath, level),
  };
}

async function loadSkillsFromDir(dir: string, level: "user" | "project"): Promise<Skill[]> {
  const skillsDir = path.join(dir, "skills");
  if (!(await isDirectory(skillsDir))) return [];

  const skills: Skill[] = [];

  // Flat markdown files
  for (const file of await listMarkdownFiles(skillsDir)) {
    const skill = await loadSkillFile(file, level);
    if (skill) skills.push(skill);
  }

  // Nested skills/<name>/SKILL.md
  let entries;
  try {
    entries = await fs.readdir(skillsDir, { withFileTypes: true });
  } catch {
    return skills;
  }

  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    if (entry.name.startsWith(".")) continue;
    const skillMd = path.join(skillsDir, entry.name, "SKILL.md");
    const skill = await loadSkillFile(skillMd, level);
    if (skill) skills.push(skill);
  }

  return skills;
}

/**
 * Load skills from user and project config.
 * Project skills override user skills of the same name.
 */
export async function discoverSkills(
  options: { cwd?: string; home?: string; stopAt?: string } = {},
): Promise<Skill[]> {
  const roots = resolveRoots(options);
  const byName = new Map<string, Skill>();

  for (const userDir of await getUserConfigDirs(roots.home)) {
    for (const skill of await loadSkillsFromDir(userDir, "user")) {
      byName.set(skill.name, skill);
    }
  }

  const projectDirs = await getProjectConfigDirs(roots.cwd, roots.stopAt);
  const ordered = [...projectDirs].sort((a, b) => b.depth - a.depth);
  for (const { dir } of ordered) {
    for (const skill of await loadSkillsFromDir(dir, "project")) {
      byName.set(skill.name, skill);
    }
  }

  return Array.from(byName.values()).sort((a, b) =>
    a.name.toLowerCase().localeCompare(b.name.toLowerCase()),
  );
}
