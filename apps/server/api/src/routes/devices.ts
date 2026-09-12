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

deviceRoutes.get("/devices/:id/stream", async (c) => {
  const id = c.req.param("id");
  const platform = (c.req.query("platform") as "ios" | "android") || (id.includes("-") ? "ios" : "android");
  const signal = c.req.raw.signal;

  const stream = new ReadableStream<Uint8Array>({
    async start(controller) {
      const boundary = "frame";
      const encoder = new TextEncoder();

      while (!signal.aborted) {
        try {
          const { data, mimeType } = await deviceService.captureStreamFrame(id, platform);
          if (signal.aborted) break;
          if (data && data.length > 0) {
            const header = `--${boundary}\r\nContent-Type: ${mimeType}\r\nContent-Length: ${data.length}\r\n\r\n`;
            controller.enqueue(encoder.encode(header));
            controller.enqueue(new Uint8Array(data));
            controller.enqueue(encoder.encode("\r\n"));
          }
        } catch {
          break;
        }
      }
      try { controller.close(); } catch {}
    },
  });

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
    const pngBuffer = await deviceService.screenshot(id, platform);
    return new Response(pngBuffer, {
      headers: { "Content-Type": "image/png" },
    });
  } catch (error) {
    return c.json({ success: false, error: error instanceof Error ? error.message : String(error) }, 500);
  }
});
