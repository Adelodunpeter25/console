/**
 * Desktop Assistant Support Routes.
 *   GET  /api/assist/:sessionId/commands — list slash commands for autocomplete
 *   GET  /api/assist/:sessionId/search   — FFF-backed fuzzy file search (@ file refs)
 * Both resolve the session's working directory as the search root.
 */
import { Hono } from "hono";
import { SlashCommandRegistry } from "../../../agent/src/commands/registry.js";
import { discoverCommands } from "../../../agent/src/systemprompt/discover-commands.js";
import { discoverSkills } from "../../../agent/src/systemprompt/discover-skills.js";
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";
import { searchFiles } from "../services/assist.service.js";
import type { SlashCommandInfo } from "@console/types";

export const assistRoutes = new Hono();
const sessionStorage = new SqliteSessionStorage();

/**
 * GET /api/assist/:sessionId/commands
 * List built-in slash commands plus discovered custom commands and skills so
 * the desktop composer can render an autocomplete dropdown.
 */
assistRoutes.get("/assist/:sessionId/commands", async (c) => {
  const sessionId = c.req.param("sessionId");
  const session = sessionStorage.loadSession(sessionId);
  const cwd = session?.header.cwd ?? process.cwd();

  const registry = new SlashCommandRegistry();
  const commands: SlashCommandInfo[] = registry.getCommands().map((cmd) => ({
    name: cmd.name,
    description: cmd.description,
    builtin: true,
  }));

  const [discovered, skills] = await Promise.all([
    discoverCommands({ cwd }),
    discoverSkills({ cwd }),
  ]);

  for (const cmd of discovered) {
    commands.push({ name: cmd.name, description: cmd.description ?? "", builtin: false });
  }
  // Skills are invokable as bare /<name>. Skip any whose name collides with a
  // built-in or custom command so the autocomplete doesn't show duplicates.
  const existing = new Set(commands.map((c) => c.name));
  for (const skill of skills.filter((s) => !s.hide)) {
    if (existing.has(skill.name)) continue;
    commands.push({ name: skill.name, description: skill.description ?? "", builtin: false });
  }

  return c.json({ success: true, data: commands });
});

/**
 * GET /api/assist/:sessionId/search?q=...
 * FFF-backed fuzzy search scoped to the session's working directory.
 * Powers the @-mention file picker in the desktop composer.
 */
assistRoutes.get("/assist/:sessionId/search", async (c) => {
  const sessionId = c.req.param("sessionId");
  const query = (c.req.query("q") ?? "").trim();
  const session = sessionStorage.loadSession(sessionId);
  const root = session?.header.cwd ?? process.cwd();

  if (!query) {
    return c.json({ success: true, data: { root, query, items: [] } });
  }

  try {
    const items = await searchFiles(root, query);
    return c.json({ success: true, data: { root, query, items } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});
