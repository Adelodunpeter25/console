/**
 * File Browser & Operations Route Handler (/api/fs/*).
 * Thin controller delegating all business logic to FsService.
 */
import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { FsService } from "@/api/src/services/fs.service.js";
import { fsWatchService } from "@/api/src/services/fswatch.service.js";
import { searchFiles } from "@/api/src/services/assist.service.js";

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
 * GET /api/fs/search — FFF-backed fuzzy file search scoped to a project root.
 * Query: root=<project dir>, q=<fuzzy query>, limit=<max results, default 20>
 * Powers the Files-screen search without the client loading the whole tree.
 */
fsRoutes.get("/search", async (c) => {
  const root = c.req.query("root");
  const query = c.req.query("q") ?? "";
  if (!root) {
    return c.json({ success: false, error: "Missing required query param: root" }, 400);
  }
  const limit = Math.min(Math.max(Number.parseInt(c.req.query("limit") ?? "20", 10) || 20, 1), 100);
  try {
    const items = await searchFiles(root, query, limit);
    return c.json({ success: true, data: items });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
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
