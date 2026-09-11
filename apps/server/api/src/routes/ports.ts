import { Hono } from "hono";
import { portRegistry } from "@/api/src/services/port-registry.service.js";

export const portRoutes = new Hono();

portRoutes.get("/ports", (c) => {
  const host = (c.req.header("host") ?? "localhost").replace(/:\d+$/, "");
  return c.json({ success: true, data: portRegistry.list(host) });
});

portRoutes.post("/ports/forward", async (c) => {
  let body: { port?: unknown };
  try {
    body = await c.req.json<{ port?: unknown }>();
  } catch {
    return c.json({ success: false, error: "Request body must be valid JSON." }, 400);
  }
  const port = typeof body.port === "number" ? body.port : Number(body.port);
  try {
    const entry = await portRegistry.forward(port);
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
  const removed = await portRegistry.remove(port);
  if (!removed) return c.json({ success: false, error: `Port ${port} is not forwarded.` }, 404);
  return c.json({ success: true, data: { port } });
});
