/**
 * Cline auth routes.
 *   GET  /api/auth/cline/status   → { loggedIn: boolean }
 *   POST /api/auth/cline/login    → body { apiKey: string } → saves after a live /v1/models probe
 *   POST /api/auth/cline/logout   → clears the stored credential
 */
import { Hono } from "hono";
import {
  CLINE_BASE_URL,
  clearClineCredential,
  loadClineCredential,
  saveClineCredential,
} from "@/providers/src/cline/index.js";

export const clineAuthRoutes = new Hono();

clineAuthRoutes.get("/cline/status", async (c) => {
  const cred = await loadClineCredential();
  return c.json({ success: true, data: { loggedIn: cred !== null } });
});

clineAuthRoutes.post("/cline/login", async (c) => {
  const body = (await c.req.json().catch(() => null)) as { apiKey?: string } | null;
  const apiKey = body?.apiKey?.trim();
  if (!apiKey) {
    return c.json({ success: false, error: "Missing apiKey." }, 400);
  }

  // Probe /v1/models with the new key before saving. Catches typos and
  // revoked keys at login time, not at first chat.
  let probeStatus = 0;
  let probeOk = false;
  try {
    const probe = await fetch(`${CLINE_BASE_URL}/models`, {
      method: "GET",
      headers: {
        Authorization: `Bearer ${apiKey}`,
        Accept: "application/json",
        "X-Title": "Console",
      },
    });
    probeStatus = probe.status;
    probeOk = probe.ok;
  } catch {
    // Network error: treat as failure
  }

  if (!probeOk) {
    return c.json(
      { success: false, error: `Cline rejected the API key (HTTP ${probeStatus}).` },
      400,
    );
  }

  await saveClineCredential({ apiKey });
  return c.json({ success: true, data: { loggedIn: true } });
});

clineAuthRoutes.post("/cline/logout", async (c) => {
  await clearClineCredential();
  return c.json({ success: true, data: { loggedIn: false } });
});