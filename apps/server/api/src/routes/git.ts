import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { GitService } from "@/api/src/services/git.service.js";
import { fsWatchService } from "@/api/src/services/fswatch.service.js";

export const gitRoutes = new Hono();
const gitService = new GitService();

/**
 * GET /api/git/status — Get repository git status summary.
 */
gitRoutes.get("/status", async (c) => {
  const repoPath = c.req.query("path");
  if (!repoPath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }
  try {
    const summary = await gitService.getGitStatus(repoPath);
    return c.json({ success: true, data: summary });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * GET /api/git/status/watch — SSE stream of git status summaries.
 *
 * Sends one `gitStatus` snapshot on subscribe, then re-computes and pushes
 * on debounced filesystem changes (via FsWatchService). One shared OS
 * watcher per repo path; per-subscriber git recompute debounced 500ms so
 * bulk writes don't spawn a git process per keystroke or hit index.lock.
 * Non-git folders get an empty clean summary and heartbeat only.
 */
gitRoutes.get("/status/watch", (c) => {
  const repoPath = c.req.query("path");
  if (!repoPath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }

  fsWatchService.watch(repoPath);

  return streamSSE(c, async (stream) => {
    const sendStatus = async () => {
      try {
        const summary = await gitService.getGitStatus(repoPath);
        await stream.writeSSE({ event: "gitStatus", data: JSON.stringify(summary) });
      } catch {
        // Client gone or git failed mid-stream — heartbeat loop exits on abort.
      }
    };

    await sendStatus();

    let debounce: ReturnType<typeof setTimeout> | undefined;
    const handler = (evt: { projectPath: string }) => {
      if (evt.projectPath !== repoPath) return;
      if (debounce) clearTimeout(debounce);
      debounce = setTimeout(() => {
        debounce = undefined;
        void sendStatus();
      }, 500);
    };

    fsWatchService.on("change", handler);
    stream.onAbort(() => {
      fsWatchService.off("change", handler);
      if (debounce) clearTimeout(debounce);
    });

    while (!stream.aborted) {
      await stream.sleep(15000);
      await stream.writeSSE({ event: "ping", data: "" });
    }
  });
});

/**
 * GET /api/git/diff — Get unified diff for the repo or a specific file.
 */
gitRoutes.get("/diff", async (c) => {
  const repoPath = c.req.query("repoPath") || c.req.query("cwd");
  const filePath = c.req.query("path") || undefined;
  if (!repoPath && !filePath) {
    return c.json(
      { success: false, error: "Query parameter 'repoPath' (or 'path') is required." },
      400,
    );
  }
  try {
    const diff = await gitService.getDiff(repoPath ?? filePath!, filePath);
    return c.json({ success: true, data: { path: filePath, diff } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * GET /api/git/branches — List local branches with the checked-out one marked.
 */
gitRoutes.get("/branches", async (c) => {
  const repoPath = c.req.query("path");
  if (!repoPath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }
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
  if (!body.path) {
    return c.json({ success: false, error: "Field 'path' is required." }, 400);
  }
  const repoPath = body.path;
  try {
    await gitService.checkoutBranch(repoPath, body.branch);
    return c.json({ success: true, data: { branch: body.branch } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});
