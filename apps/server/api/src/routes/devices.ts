import { Hono } from "hono";
import { deviceManager } from "@/api/src/services/device/index.js";
import type { DeviceActionRequest, DeviceOpenAppRequest } from "@console/types";

export const deviceRoutes = new Hono();

deviceRoutes.get("/devices", async (c) => {
  try {
    const devices = await deviceManager.listDevices();
    return c.json({ success: true, data: devices });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.get("/devices/diagnostics", async (c) => {
  try {
    const diagnostics = await deviceManager.getDiagnostics();
    return c.json({ success: true, data: diagnostics });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.post("/devices/:id/boot", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  try {
    await deviceManager.bootDevice(id, platform);
    return c.json({ success: true, data: { id, state: "booted" } });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.post("/devices/:id/shutdown", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  try {
    await deviceManager.shutdownDevice(id, platform);
    return c.json({ success: true, data: { id, state: "shutdown" } });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.post("/devices/:id/open-app", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  const body = await c.req.json<DeviceOpenAppRequest>();
  try {
    await deviceManager.openApp(id, platform, body.app);
    return c.json({ success: true, data: { id, app: body.app } });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.post("/devices/:id/interact", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  const body = await c.req.json<DeviceActionRequest>();
  try {
    await deviceManager.interact(id, platform, body);
    return c.json({ success: true, data: { success: true } });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.get("/devices/:id/stream", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  const signal = c.req.raw.signal;

  if (platform === "android") {
    const stream = deviceManager.createH264Stream(id, signal);
    return new Response(stream, {
      headers: {
        "Content-Type": "video/h264",
        "Cache-Control": "no-cache, no-store, must-revalidate",
        "Connection": "keep-alive",
        "Pragma": "no-cache",
      },
    });
  }

  const stream = await deviceManager.createIosStream(id, signal);
  return new Response(stream, {
    headers: {
      "Content-Type": "multipart/x-mixed-replace; boundary=frame",
      "Cache-Control": "no-cache, no-store, must-revalidate",
      "Connection": "keep-alive",
      "Pragma": "no-cache",
    },
  });
});

deviceRoutes.get("/devices/:id/screenshot", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  try {
    const pngBuffer = await deviceManager.screenshot(id, platform);
    return new Response(pngBuffer, {
      headers: { "Content-Type": "image/png" },
    });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});
