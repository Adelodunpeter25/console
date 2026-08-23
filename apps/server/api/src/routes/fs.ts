/**
 * File Browser & Operations Route Handler (/api/fs/*).
 * Thin controller delegating all business logic to FsService.
 */
import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { FsService } from "../services/fs.service.js";
import { fsWatchService } from "../services/fswatch.service.js";

export const fsRoutes = new Hono();
const fsService = new FsService();

/**
 * GET /api/fs/browse — Browse system directories for file picker UI.
 */
fsRoutes.get("/browse", async (c) => {
  const dirPath = c.req.query("path");
  try {
    const result = await fsService.browseDirectory(dirPath);
    return c.json({ success: true, data: result });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * POST /api/fs/pick-folder — Opens native macOS Finder folder picker dialog.
 */
fsRoutes.post("/pick-folder", async (c) => {
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  try {
    const script = `osascript -e 'POSIX path of (choose folder with prompt "Select Project Folder")'`;
    const { stdout } = await execAsync(script);
    const selectedPath = stdout.trim().replace(/\/$/, "");
    return c.json({ success: true, data: { path: selectedPath } });
  } catch (err) {
    // osascript exits non-zero when the user dismisses the dialog; the
    // "User canceled" marker (-128) is not a failure, just a cancelled pick.
    const errMsg = err instanceof Error ? err.message : String(err);
    const cancelled = /-128|User canceled|user cancelled/i.test(errMsg);
    return c.json(
      cancelled
        ? { success: false, cancelled: true, error: "Folder selection cancelled." }
        : { success: false, error: "Folder selection failed." },
      400
    );
  }
});

/**
 * POST /api/fs/pick-file — Opens a native macOS file picker dialog and
 * returns the selected file's path. Dismissing the dialog is a cancelled
 * pick, not a failure.
 */
fsRoutes.post("/pick-file", async (c) => {
  const { exec } = await import("node:child_process");
  const { promisify } = await import("node:util");
  const execAsync = promisify(exec);

  try {
    const script = `osascript -e 'POSIX path of (choose file with prompt "Select an image")'`;
    const { stdout } = await execAsync(script);
    const selectedPath = stdout.trim();
    return c.json({ success: true, data: { path: selectedPath } });
  } catch (err) {
    const errMsg = err instanceof Error ? err.message : String(err);
    const cancelled = /-128|User canceled|user cancelled/i.test(errMsg);
    return c.json(
      cancelled
        ? { success: false, cancelled: true, error: "File selection cancelled." }
        : { success: false, error: "File selection failed." },
      400
    );
  }
});

/**
 * GET /api/fs/entries — List all entries recursively for building a file tree.
 * Query: path=<project root>, depth=<max depth, default 6>, hidden=<include dotfiles>
 */
fsRoutes.get("/entries", async (c) => {
  const dirPath = c.req.query("path");
  if (!dirPath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }
  const maxDepth = Number.parseInt(c.req.query("depth") || "6", 10);
  const showHidden = c.req.query("hidden") === "true";

  try {
    const entries = await fsService.listAllEntries(dirPath, maxDepth, showHidden);
    return c.json({ success: true, data: entries });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * GET /api/fs/tree — Return structured directory tree summary.
 */
fsRoutes.get("/tree", async (c) => {
  const dirPath = c.req.query("path") || process.cwd();
  const maxDepth = Number.parseInt(c.req.query("depth") || "3", 10);

  try {
    const treeFormatted = await fsService.getDirectoryTree(dirPath, maxDepth);
    return c.json({
      success: true,
      data: { path: dirPath, treeFormatted },
    });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * GET /api/fs/file — Read file content.
 */
fsRoutes.get("/file", async (c) => {
  const filePath = c.req.query("path");
  if (!filePath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }

  const startLine = c.req.query("startLine")
    ? Number.parseInt(c.req.query("startLine")!, 10)
    : undefined;
  const endLine = c.req.query("endLine") ? Number.parseInt(c.req.query("endLine")!, 10) : undefined;

  try {
    const content = await fsService.readFileContent(filePath, startLine, endLine);
    return c.json({
      success: true,
      data: { path: filePath, content },
    });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * POST /api/fs/file — Create or save file content.
 */
fsRoutes.post("/file", async (c) => {
  const body = await c.req.json<{ path: string; content: string }>();
  if (!body.path) {
    return c.json({ success: false, error: "Field 'path' is required." }, 400);
  }

  try {
    const resultText = await fsService.writeFileContent(body.path, body.content || "");
    return c.json({
      success: true,
      data: { path: body.path, message: resultText },
    });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * DELETE /api/fs/file — Delete a file.
 */
fsRoutes.delete("/file", async (c) => {
  const filePath = c.req.query("path");
  if (!filePath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }

  try {
    await fsService.deleteFile(filePath);
    return c.json({ success: true, data: { path: filePath, deleted: true } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * POST /api/fs/dir — Create a new directory.
 */
fsRoutes.post("/dir", async (c) => {
  const body = await c.req.json<{ path: string }>();
  if (!body.path) {
    return c.json({ success: false, error: "Field 'path' is required." }, 400);
  }

  try {
    await fsService.createDirectory(body.path);
    return c.json({ success: true, data: { path: body.path, created: true } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * DELETE /api/fs/dir — Delete a directory.
 */
fsRoutes.delete("/dir", async (c) => {
  const dirPath = c.req.query("path");
  if (!dirPath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }

  try {
    await fsService.deleteDirectory(dirPath);
    return c.json({ success: true, data: { path: dirPath, deleted: true } });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});

/**
 * GET /api/fs/watch — SSE stream for real-time filesystem change events.
 */
fsRoutes.get("/watch", (c) => {
  const projectPath = c.req.query("path");
  if (!projectPath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }

  fsWatchService.watch(projectPath);

  return streamSSE(c, async (stream) => {
    const handler = (evt: any) => {
      if (evt.projectPath === projectPath) {
        stream.writeSSE({
          event: "fsChange",
          data: JSON.stringify(evt),
        });
      }
    };

    fsWatchService.on("change", handler);

    stream.onAbort(() => {
      fsWatchService.off("change", handler);
    });

    // Keep connection alive with heartbeat
    while (!stream.aborted) {
      await stream.sleep(15000);
      await stream.writeSSE({ event: "ping", data: "" });
    }
  });
});
