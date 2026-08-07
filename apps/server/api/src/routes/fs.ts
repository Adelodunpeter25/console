/**
 * File Browser & Operations Route Handler (/api/fs/*).
 * Thin controller delegating all business logic to FsService.
 */
import { Hono } from "hono";
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
  } catch {
    return c.json({ success: false, error: "Folder selection cancelled or failed." }, 400);
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
 * GET /api/fs/watch — SSE stream for real-time filesystem change events.
 */
fsRoutes.get("/watch", (c) => {
  const projectPath = c.req.query("path");
  if (!projectPath) {
    return c.json({ success: false, error: "Query parameter 'path' is required." }, 400);
  }

  fsWatchService.watch(projectPath);

  return c.streamSSE(async (stream) => {
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
