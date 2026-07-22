/**
 * Remote OAuth Auth Routes (/api/auth/*).
 * Controller delegating business logic to AuthService.
 */
import { Hono } from "hono";
import { AuthService } from "../services/auth.service.js";
import type { OAuthCallbackDto, OAuthLoginUrlDto } from "../types/index.js";

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
