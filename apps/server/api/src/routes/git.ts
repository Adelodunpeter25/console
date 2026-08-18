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

/**
 * GET /api/git/branches — List local branches with the checked-out one marked.
 */
gitRoutes.get("/branches", async (c) => {
  const repoPath = c.req.query("path") || process.cwd();
  try {
    const branches = await gitService.listBranches(repoPath);
    return c.json({ success: true, data: branches });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * POST /api/git/checkout — Check out an existing local branch.
 */
gitRoutes.post("/checkout", async (c) => {
  const body = await c.req.json<{ path?: string; branch: string }>();
  if (!body.branch) {
    return c.json({ success: false, error: "Field 'branch' is required." }, 400);
  }
  const repoPath = body.path || process.cwd();
  try {
    await gitService.checkoutBranch(repoPath, body.branch);
    return c.json({ success: true, data: { branch: body.branch } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});
