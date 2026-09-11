import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { ProjectScriptsService } from "@/api/src/services/project-scripts/service.js";

export const projectScriptRoutes = new Hono();
const service = new ProjectScriptsService();

projectScriptRoutes.get("/projects/:projectId/scripts", async (c) => {
  try {
    return c.json({ success: true, data: await service.list(c.req.param("projectId")) });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 400);
  }
});

projectScriptRoutes.post("/projects/:projectId/scripts/:scriptId/runs", async (c) => {
  try {
    return c.json({ success: true, data: await service.run(c.req.param("projectId"), c.req.param("scriptId")) }, 201);
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 400);
  }
});

projectScriptRoutes.get("/projects/:projectId/scripts/runs", (c) =>
  c.json({ success: true, data: service.listRuns(c.req.param("projectId")) }),
);

projectScriptRoutes.get("/projects/:projectId/scripts/runs/:runId", (c) => {
  const run = service.getRun(c.req.param("projectId"), c.req.param("runId"));
  return run ? c.json({ success: true, data: run }) : c.json({ success: false, error: "Run not found." }, 404);
});

projectScriptRoutes.post("/projects/:projectId/scripts/runs/:runId/stop", (c) => {
  const stopped = service.stop(c.req.param("projectId"), c.req.param("runId"));
  return stopped ? c.json({ success: true, data: { stopped: true } }) : c.json({ success: false, error: "Run not found or already stopped." }, 404);
});

projectScriptRoutes.get("/projects/:projectId/scripts/runs/:runId/stream", (c) => {
  const projectId = c.req.param("projectId");
  const runId = c.req.param("runId");
  if (!service.getRun(projectId, runId)) return c.json({ success: false, error: "Run not found." }, 404);
  return streamSSE(c, async (stream) => {
    let closed = false;
    const unsubscribe = service.subscribe(projectId, runId, async (event) => {
      if (closed) return;
      try { await stream.writeSSE({ event: event.type, data: JSON.stringify(event) }); } catch { closed = true; }
    });
    await new Promise<void>((resolve) => {
      const timer = setInterval(() => {
        if (!service.getRun(projectId, runId)?.status.includes("running")) { clearInterval(timer); resolve(); }
      }, 100);
    });
    unsubscribe?.();
  });
});
