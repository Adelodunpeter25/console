/**
 * Remote OAuth Auth Routes (/api/auth/*).
 * Controller delegating business logic to AuthService.
 */
import { Hono } from "hono";
import { AuthService } from "../services/auth.service.js";
import type { OAuthCallbackDto, OAuthLoginUrlDto, ProjectIdDto } from "../types/index.js";

export const authRoutes = new Hono();
const authService = new AuthService();

/**
 * GET /api/auth/status — Check provider login credentials status.
 */
authRoutes.get("/status", async (c) => {
  const status = await authService.getAuthStatus();
  return c.json({
    success: true,
    data: status,
  });
});

/**
 * POST /api/auth/login/url — Generate remote OAuth URL for mobile/desktop browser.
 */
authRoutes.post("/login/url", async (c) => {
  const body = await c.req.json<OAuthLoginUrlDto>();
  const provider = body.provider ?? "antigravity";
  const result = authService.getLoginUrl(provider);

  return c.json({
    success: true,
    data: result,
  });
});

/**
 * POST /api/auth/login/callback — Receive authorization code from client, perform token exchange on server.
 */
authRoutes.post("/login/callback", async (c) => {
  const body = await c.req.json<OAuthCallbackDto>();
  const { provider, code } = body;

  if (!code) {
    return c.json({ success: false, error: "Authorization 'code' is required." }, 400);
  }

  try {
    const data = await authService.handleCallback(provider, code);
    return c.json({
      success: true,
      data,
    });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 500);
  }
});

/**
 * GET /api/auth/project-id/:provider — Get the configured Google Cloud project ID.
 */
authRoutes.get("/project-id/:provider", async (c) => {
  const provider = c.req.param("provider") as "gemini" | "antigravity";
  if (provider !== "gemini" && provider !== "antigravity") {
    return c.json({ success: false, error: "Invalid provider." }, 400);
  }
  const projectId = await authService.getProjectId(provider);
  return c.json({ success: true, data: { projectId } });
});

/**
 * POST /api/auth/project-id — Save the configured Google Cloud project ID.
 */
authRoutes.post("/project-id", async (c) => {
  const body = await c.req.json<ProjectIdDto>();
  console.log("POST /project-id body:", body);
  const { provider, projectId } = body;
  if (provider !== "gemini" && provider !== "antigravity") {
    return c.json({ success: false, error: "Invalid provider." }, 400);
  }
  await authService.setProjectId(provider, projectId);
  return c.json({ success: true, data: { provider, projectId: projectId ?? null } });
});
