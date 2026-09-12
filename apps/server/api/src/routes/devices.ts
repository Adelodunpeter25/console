import { Hono } from "hono";
import { deviceService } from "@/api/src/services/device.service.js";
import type { DeviceActionRequest, DeviceOpenAppRequest } from "@console/types";

export const deviceRoutes = new Hono();

deviceRoutes.get("/devices", async (c) => {
  try {
    const devices = await deviceService.listDevices();
    return c.json({ success: true, data: devices });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.get("/devices/diagnostics", async (c) => {
  try {
    const diagnostics = await deviceService.getDiagnostics();
    return c.json({ success: true, data: diagnostics });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.post("/devices/:id/boot", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  try {
    await deviceService.bootDevice(id, platform);
    return c.json({ success: true, data: { id, state: "booted" } });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.post("/devices/:id/shutdown", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  try {
    await deviceService.shutdownDevice(id, platform);
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
    await deviceService.openApp(id, platform, body.app);
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
    await deviceService.interact(id, platform, body);
    return c.json({ success: true, data: { success: true } });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});

deviceRoutes.get("/devices/:id/screenshot", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  try {
    const pngBuffer = await deviceService.screenshot(id, platform);
    return new Response(pngBuffer, {
      headers: { "Content-Type": "image/png" },
    });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});
