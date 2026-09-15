import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { portRegistry } from "@/api/src/services/port-registry.service.js";

export const portRoutes = new Hono();

portRoutes.get("/ports/stream", (c) => {
  const host = (c.req.header("host") ?? "localhost").replace(/:\d+$/, "");
  const projectId = c.req.query("projectId") ?? undefined;
  return streamSSE(c, async (stream) => {
    const sendSnapshot = async (ports = portRegistry.snapshot(host, projectId)) => {
      await stream.writeSSE({ event: "ports", data: JSON.stringify(ports) });
    };
    const handler = () => {
      void sendSnapshot(portRegistry.snapshot(host, projectId));
    };

    portRegistry.on("change", handler);
    await sendSnapshot();
    stream.onAbort(() => {
      portRegistry.off("change", handler);
    });

    while (!stream.aborted) {
      await stream.sleep(15000);
      await stream.writeSSE({ event: "ping", data: "" });
    }
  });
});

portRoutes.get("/ports", async (c) => {
  const host = (c.req.header("host") ?? "localhost").replace(/:\d+$/, "");
  const projectId = c.req.query("projectId") ?? undefined;
  return c.json({ success: true, data: await portRegistry.list(host, projectId) });
});

portRoutes.post("/ports/forward", async (c) => {
  let body: { port?: unknown; projectId?: unknown };
  try {
    body = await c.req.json<{ port?: unknown; projectId?: unknown }>();
  } catch {
    return c.json({ success: false, error: "Request body must be valid JSON." }, 400);
  }
  const port = typeof body.port === "number" ? body.port : Number(body.port);
  const projectId = typeof body.projectId === "string" ? body.projectId : undefined;
  try {
    const entry = await portRegistry.forward(port, projectId);
    return c.json({ success: true, data: entry });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 400);
  }
});

portRoutes.delete("/ports/:port", async (c) => {
  const port = Number(c.req.param("port"));
  if (!Number.isInteger(port)) {
    return c.json({ success: false, error: "Port must be an integer." }, 400);
  }
  const projectId = c.req.query("projectId") ?? undefined;
  const removed = await portRegistry.remove(port, projectId);
  if (!removed) return c.json({ success: false, error: `Port ${port} is not forwarded.` }, 404);
  return c.json({ success: true, data: { port } });
});
