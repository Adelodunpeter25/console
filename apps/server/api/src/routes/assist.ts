/**
 * Desktop Assistant Support Routes.
 *   GET  /api/assist/:sessionId/commands / GET /api/assist/commands — list slash commands for autocomplete
 *   GET  /api/assist/:sessionId/search   / GET /api/assist/search   — FFF-backed fuzzy file search (@ file refs)
 * Both resolve the session's working directory as the search root.
 */
import { Hono, type Context } from "hono";
import { SlashCommandRegistry } from "@/agent/src/commands/registry.js";
import { discoverCommands } from "@/agent/src/systemprompt/discover-commands.js";
import { discoverSkills } from "@/agent/src/systemprompt/discover-skills.js";
import { SqliteSessionStorage } from "@/agent/src/session/storage.js";
import { searchFiles } from "@/api/src/services/assist.service.js";
import type { SlashCommandInfo } from "@console/types";

export const assistRoutes = new Hono();
const sessionStorage = new SqliteSessionStorage();

async function handleCommands(c: Context) {
  const sessionId = c.req.param("sessionId") ?? c.req.query("sessionId") ?? "";
  const session = sessionId ? sessionStorage.loadSession(sessionId) : null;
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
}

async function handleSearch(c: Context) {
  const sessionId = c.req.param("sessionId") ?? c.req.query("sessionId") ?? "";
  const query = (c.req.query("q") ?? c.req.query("query") ?? "").trim();
  const session = sessionId ? sessionStorage.loadSession(sessionId) : null;
  const root = c.req.query("root") ?? session?.header.cwd ?? process.cwd();

  try {
    // When query is empty (user just typed "@") fall back to a broad scan so
    // the desktop autocomplete can show something instead of a blank popup.
    const items = await searchFiles(root, query || ".");
    return c.json({ success: true, data: { root, query, items } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
}

assistRoutes.get("/assist/:sessionId/commands", handleCommands);
assistRoutes.get("/assist/commands", handleCommands);
assistRoutes.get("/assist/:sessionId/search", handleSearch);
assistRoutes.get("/assist/search", handleSearch);
assistRoutes.get("/assist/files", handleSearch);
