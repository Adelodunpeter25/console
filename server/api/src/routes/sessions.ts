/**
 * Session Persistence CRUD Routes (/api/sessions/*).
 * Controller delegating business logic to SessionService.
 */
import { Hono } from "hono";
import { SessionService } from "../services/session.service.js";
import type { CreateSessionDto, UpdateSessionDto } from "../types/index.js";

export const sessionRoutes = new Hono();
const sessionService = new SessionService();

/**
 * GET /api/sessions — List saved sessions (optionally filtered by cwd).
 */
sessionRoutes.get("/sessions", (c) => {
  const cwd = c.req.query("cwd");
  const sessions = sessionService.listSessions(cwd);
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
 * GET /api/sessions/:id — Load session header and complete message history.
 */
sessionRoutes.get("/sessions/:id", (c) => {
  const id = c.req.param("id");
  const session = sessionService.getSession(id);

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
