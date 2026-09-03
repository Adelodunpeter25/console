/**
 * Desktop Assistant Support Routes.
 *   GET  /api/assist/:sessionId/commands / GET /api/assist/commands — list skills for slash autocomplete
 *   GET  /api/assist/:sessionId/search   / GET /api/assist/search   — FFF-backed fuzzy file search (@ file refs)
 * Both resolve the session's working directory as the search root.
 */
import { Hono, type Context } from "hono";
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

  const skills = await discoverSkills({ cwd });

  const commands: SlashCommandInfo[] = skills
    .filter((s) => !s.hide)
    .map((skill) => ({
      name: skill.name,
      description: skill.description ?? "",
      builtin: false,
    }));

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
