/**
 * File Browser & Operations Route Handler (/api/fs/*).
 * Thin controller delegating all business logic to FsService.
 */
import { Hono } from "hono";
import { FsService } from "../services/fs.service.js";

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
 * DELETE /api/fs/dir — Delete a directory recursively.
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
