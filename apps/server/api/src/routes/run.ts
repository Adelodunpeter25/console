/**
 * Real-Time Agent SSE Streaming & Abort Routes (/api/sessions/:id/run, /api/sessions/:id/abort).
 * Controller delegating business logic to RunService.
 */
import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { RunService } from "../services/run.service.js";
import type { AnswerQuestionDto, ApproveToolPermissionDto, RunPromptDto } from "../types/index.js";

export const runRoutes = new Hono();
const runService = new RunService();

/**
 * POST /api/sessions/:id/run — Real-time SSE agent execution streaming.
 */
runRoutes.post("/sessions/:id/run", async (c) => {
  const sessionId = c.req.param("id");
  const body = await c.req.json<RunPromptDto>();
  const prompt = body.prompt?.trim();

  if (!prompt) {
    return c.json({ success: false, error: "Field 'prompt' is required." }, 400);
  }

  return streamSSE(c, async (sseStream) => {
    try {
      await runService.runAgentStream(sessionId, body, async (event) => {
        await sseStream.writeSSE({
          event: event.type,
          data: JSON.stringify(event),
        });
      });
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : String(err);
      await sseStream.writeSSE({
        event: "error",
        data: JSON.stringify({ type: "error", error: { message: errorMsg } }),
      });
    }
  });
});

/**
 * POST /api/sessions/:id/abort — Abort an active run for a session.
 */
runRoutes.post("/sessions/:id/abort", (c) => {
  const sessionId = c.req.param("id");
  const aborted = runService.abortRun(sessionId);

  if (!aborted) {
    return c.json(
      { success: false, error: `No active run found for session '${sessionId}'.` },
      404,
    );
  }

  return c.json({
    success: true,
    data: { sessionId, aborted: true },
  });
});

/**
 * POST /api/sessions/:id/answer — Answer a pending agent question.
 */
runRoutes.post("/sessions/:id/answer", async (c) => {
  const sessionId = c.req.param("id");
  const body = await c.req.json<AnswerQuestionDto>();
  const ok = runService.answerQuestion(sessionId, body.requestId, body.answer);
  if (!ok) {
    return c.json(
      { success: false, error: `No pending question for requestId '${body.requestId}'.` },
      404,
    );
  }
  return c.json({ success: true, data: { answered: true } });
});

/**
 * POST /api/sessions/:id/approve — Approve or deny a pending tool permission request.
 */
runRoutes.post("/sessions/:id/approve", async (c) => {
  const sessionId = c.req.param("id");
  const body = await c.req.json<ApproveToolPermissionDto>();
  const ok = runService.approvePermission(sessionId, body.requestId, body.allow);
  if (!ok) {
    return c.json(
      { success: false, error: `No pending permission for requestId '${body.requestId}'.` },
      404,
    );
  }
  return c.json({ success: true, data: { approved: body.allow } });
});
