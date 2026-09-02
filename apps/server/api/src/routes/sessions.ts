/**
 * Session Persistence CRUD Routes (/api/sessions/*).
 * Controller delegating business logic to SessionService.
 */
import { Hono } from "hono";
import { SessionService } from "@/api/src/services/session.service.js";
import type { CreateSessionDto, UpdateSessionDto } from "@/api/src/types/index.js";

export const sessionRoutes = new Hono();
const sessionService = new SessionService();

/**
 * GET /api/sessions — List saved sessions (optionally filtered by cwd).
 */
sessionRoutes.get("/sessions", (c) => {
  const cwd = c.req.query("cwd");
  const projectId = c.req.query("projectId");
  const onlyDeleted = c.req.query("onlyDeleted") === "true";
  const sessions = sessionService.listSessions(cwd, projectId, onlyDeleted);
  return c.json({
    success: true,
    data: sessions,
  });
});

/**
 * POST /api/sessions — Create a new session.
 */
sessionRoutes.post("/sessions", async (c) => {
  const body = await c.req.json<CreateSessionDto>();
  const header = sessionService.createSession(body);
  return c.json({
    success: true,
    data: header,
  });
});

/**
 * GET /api/sessions/:id — Load session header and a page of message history.
 */
sessionRoutes.get("/sessions/:id", (c) => {
  const id = c.req.param("id");
  const rawLimit = c.req.query("limit");
  const rawBefore = c.req.query("before");
  const limit = rawLimit === undefined ? 50 : Number(rawLimit);
  const before = rawBefore === undefined ? undefined : Number(rawBefore);

  if (
    !Number.isSafeInteger(limit) ||
    limit < 1 ||
    (before !== undefined && (!Number.isSafeInteger(before) || before < 1))
  ) {
    return c.json(
      { success: false, error: "'limit' and 'before' must be positive integers." },
      400,
    );
  }

  const session = sessionService.getSession(id, { limit, before });

  if (!session) {
    return c.json({ success: false, error: `Session '${id}' not found.` }, 404);
  }

  return c.json({
    success: true,
    data: session,
  });
});

/**
 * PATCH /api/sessions/:id — Update session title or model.
 */
sessionRoutes.patch("/sessions/:id", async (c) => {
  const id = c.req.param("id");
  const body = await c.req.json<UpdateSessionDto>();

  const updatedHeader = sessionService.updateSession(id, body);
  if (!updatedHeader) {
    return c.json({ success: false, error: `Session '${id}' not found.` }, 404);
  }

  return c.json({
    success: true,
    data: updatedHeader,
  });
});

/**
 * DELETE /api/sessions/:id — Delete a session and its message history.
 */
sessionRoutes.delete("/sessions/:id", (c) => {
  const id = c.req.param("id");
  const deleted = sessionService.deleteSession(id);

  if (!deleted) {
    return c.json({ success: false, error: `Session '${id}' not found.` }, 404);
  }

  return c.json({
    success: true,
    data: { id, deleted: true },
  });
});

/**
 * POST /api/sessions/:id/restore — Restore a soft-deleted session.
 */
sessionRoutes.post("/sessions/:id/restore", (c) => {
  const id = c.req.param("id");
  const restored = sessionService.restoreSession(id);

  if (!restored) {
    return c.json({ success: false, error: `Session '${id}' not found.` }, 404);
  }

  return c.json({
    success: true,
    data: { id, restored: true },
  });
});

/** DELETE /api/sessions/:id/permanent — Irreversibly delete a soft-deleted session. */
sessionRoutes.delete("/sessions/:id/permanent", (c) => {
  const id = c.req.param("id");
  const deleted = sessionService.permanentlyDeleteSession(id);

  if (!deleted) {
    return c.json(
      { success: false, error: `Deleted session '${id}' not found.` },
      404,
    );
  }

  return c.json({
    success: true,
    data: { id, permanentlyDeleted: true },
  });
});

/**
 * GET /api/sessions/:id/changes — Get recorded file changes for a session.
 */
sessionRoutes.get("/sessions/:id/changes", (c) => {
  const id = c.req.param("id");
  const changes = sessionService.getSessionFileChanges(id);
  return c.json({
    success: true,
    data: changes,
  });
});

/**
 * GET /api/sessions/:id/todos — Get persisted todos for a session.
 */
sessionRoutes.get("/sessions/:id/todos", (c) => {
  const id = c.req.param("id");
  const todos = sessionService.getSessionTodos(id);
  return c.json({
    success: true,
    data: todos,
  });
});

/**
 * GET /api/sessions/:id/subagents — Get persisted subagents for a session.
 */
sessionRoutes.get("/sessions/:id/subagents", (c) => {
  const id = c.req.param("id");
  const subagents = sessionService.getSessionSubagents(id);
  return c.json({
    success: true,
    data: subagents,
  });
});

