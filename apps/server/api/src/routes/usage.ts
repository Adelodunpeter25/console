/**
 * Usage Quota Routes (/api/usage, /api/providers/:id/usage).
 */
import { Hono } from "hono";
import { UsageService } from "@/api/src/services/usage.service.js";
import type { ProviderId } from "@console/types";

export const usageRoutes = new Hono();
const usageService = new UsageService();

/**
 * GET /api/usage — Aggregated usage for all quota-backed providers.
 */
usageRoutes.get("/usage", async (c) => {
  const data = await usageService.getAllUsage(c.req.raw.signal);
  return c.json({ success: true, data });
});

/**
 * GET /api/providers/:id/usage — Usage for a single provider.
 */
usageRoutes.get("/providers/:id/usage", async (c) => {
  const providerId = c.req.param("id") as ProviderId;
  if (providerId !== "gemini" && providerId !== "antigravity" && providerId !== "codex") {
    return c.json({ success: false, error: `Invalid provider '${providerId}' for usage.` }, 400);
  }

  const report = await usageService.getUsage(providerId, c.req.raw.signal);
  return c.json({
    success: true,
    data: report,
  });
});
