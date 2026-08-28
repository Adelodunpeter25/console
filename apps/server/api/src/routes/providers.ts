/**
 * Provider & Model Catalog Routes (/api/providers/*).
 * Controller delegating business logic to ProviderService.
 */
import { Hono } from "hono";
import { ProviderService } from "@/api/src/services/provider.service.js";
import type { ProviderId } from "@console/types";

export const providerRoutes = new Hono();
const providerService = new ProviderService();

/**
 * GET /api/providers — List all registered LLM providers.
 */
providerRoutes.get("/providers", (c) => {
  const providers = providerService.getProviders();
  return c.json({
    success: true,
    data: providers,
  });
});

/**
 * GET /api/providers/:id/models — Fetch dynamic models for provider.
 */
providerRoutes.get("/providers/:id/models", async (c) => {
  const providerId = c.req.param("id") as ProviderId;
  if (
    providerId !== "gemini" &&
    providerId !== "antigravity" &&
    providerId !== "opencode" &&
    providerId !== "codex"
  ) {
    return c.json({ success: false, error: `Invalid provider '${providerId}'.` }, 400);
  }

  try {
    const models = await providerService.getModels(providerId);
    return c.json({
      success: true,
      data: {
        provider: providerId,
        models,
      },
    });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 500);
  }
});
