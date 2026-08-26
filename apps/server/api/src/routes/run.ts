/**
 * Real-Time Agent SSE Streaming & Abort Routes (/api/sessions/:id/run, /api/sessions/:id/abort).
 * Controller delegating business logic to RunService.
 */
import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { randomUUID } from "node:crypto";
import { RunService } from "@/api/src/services/run.service.js";
import { extractErrorMessage } from "@/agent/src/utils/error.js";
import type { AnswerQuestionDto, ApproveToolPermissionDto, RunPromptDto } from "@/api/src/types/index.js";
import type { AgentSessionEvent } from "@console/types";

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
    // The agent run must outlive the desktop/mobile client. If a window is
    // closed while a tool is running, writing to the disconnected SSE stream
    // can fail; that must not cancel the server-side run before its completed
    // tool results are persisted.
    let clientConnected = true;
    try {
      await runService.runAgentStream(sessionId, body, async (event) => {
        if (!clientConnected) return;
        try {
          await sseStream.writeSSE({
            event: event.type,
            data: JSON.stringify(event),
          });
        } catch {
          clientConnected = false;
        }
      });
    } catch (err) {
      const errorMsg = extractErrorMessage(err);
      if (clientConnected) {
        try {
          await sseStream.writeSSE({
            event: "error",
            data: JSON.stringify({ type: "error", error: { message: errorMsg } }),
          });
        } catch {
          // The client may have disconnected while the run was failing.
        }
      }
    }
  });
});

/**
 * GET /api/sessions/:id/run/stream — Attach to an in-flight agent run.
 *
 * Lets any surface (mobile re-attach, desktop follow-along) subscribe live to
 * a run started elsewhere. Query params:
 *   since=<seq> — replay buffered events newer than seq first (a
 *                 `streamReset` frame precedes the batch so clients clear
 *                 their streaming buffers before applying the replay).
 * Omit `since` to go live immediately (client loads history from SQLite).
 * Responds 409 when no run is active for the session.
 */
runRoutes.get("/sessions/:id/run/stream", (c) => {
  const sessionId = c.req.param("id");

  const sinceRaw = c.req.query("since");
  let since: number | undefined;
  if (sinceRaw !== undefined && sinceRaw !== "") {
    since = Number.parseInt(sinceRaw, 10);
    if (Number.isNaN(since) || since < 0) {
      return c.json(
        { success: false, error: "Field 'since' must be a non-negative integer." },
        400,
      );
    }
  }

  if (!RunService.isRunActive(sessionId)) {
    return c.json(
      { success: false, error: `No active run for session '${sessionId}'.` },
      409,
    );
  }

  return streamSSE(c, async (sseStream) => {
    const writeFrame = async (seq: number | null, event: AgentSessionEvent) => {
      await sseStream.writeSSE({
        ...(seq !== null ? { id: String(seq) } : {}),
        event: event.type,
        data: JSON.stringify(event),
      });
    };

    try {
      if (since !== undefined) {
        // Tell the client to clear its streaming buffers before the replay
        // batch — coalesced snapshots would otherwise duplicate text it
        // already accumulated before disconnecting.
        await writeFrame(null, { type: "streamReset" });
      }

      const subscriberId = randomUUID();
      const attached = runService.subscribeToActiveRun(
        sessionId,
        {
          id: subscriberId,
          extendedFrames: true,
          deliver: (seq, event) => writeFrame(seq, event),
          ping: () => {
            void sseStream.write(": ping\n\n");
          },
        },
        since,
      );

      if (!attached) {
        // The run settled between the 409 pre-check and registration.
        await writeFrame(null, {
          type: "error",
          error: { message: `No active run for session '${sessionId}'.` },
        });
        return;
      }

      // Hold the stream open until the run settles; bail early when the
      // client disconnects (Bun aborts the request signal on socket close).
      await new Promise<void>((resolve) => {
        let finished = false;
        const finish = () => {
          if (finished) return;
          finished = true;
          runService.unsubscribeFromActiveRun(sessionId, subscriberId);
          resolve();
        };
        void runService.waitForRunSettle(sessionId).then(finish);
        const signal = c.req.raw.signal;
        if (signal.aborted) finish();
        else signal.addEventListener("abort", finish, { once: true });
      });
    } catch {
      // Socket died during setup/teardown — nothing further to send.
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
