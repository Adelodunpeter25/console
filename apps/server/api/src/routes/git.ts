import { Hono } from "hono";
import { GitService } from "../services/git.service.js";

export const gitRoutes = new Hono();
const gitService = new GitService();

/**
 * GET /api/git/status — Get repository git status summary.
 */
gitRoutes.get("/status", async (c) => {
  const repoPath = c.req.query("path") || process.cwd();
  try {
    const summary = await gitService.getGitStatus(repoPath);
    return c.json({ success: true, data: summary });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});
