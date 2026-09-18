import { Hono } from "hono";
import { isConsoleModelRole, loadSettings, saveSettings, type ConsoleModelRole } from "@/agent/src/service/model-roles.js";

export const settingsRoutes = new Hono();

settingsRoutes.get("/settings", async (c) => c.json({ success: true, data: await loadSettings() }));

settingsRoutes.patch("/settings", async (c) => {
  const body = await c.req.json<unknown>();
  if (!body || typeof body !== "object" || !Object.hasOwn(body, "modelRoles")) {
    return c.json({ success: false, error: "modelRoles is required." }, 400);
  }
  const raw = (body as { modelRoles?: unknown }).modelRoles;
  if (!raw || typeof raw !== "object") return c.json({ success: false, error: "modelRoles must be an object." }, 400);
  // Only roles present in the body are touched (set, or cleared if
  // null/empty) — a role the caller doesn't mention is left as-is.
  const patch: Partial<Record<ConsoleModelRole, string | null>> = {};
  for (const [role, model] of Object.entries(raw)) {
    if (!isConsoleModelRole(role)) {
      return c.json({ success: false, error: `Invalid model role '${role}'.` }, 400);
    }
    if (model === null || model === undefined) {
      patch[role] = null;
      continue;
    }
    if (typeof model !== "string") {
      return c.json({ success: false, error: `Value for model role '${role}' must be a string.` }, 400);
    }
    patch[role] = model;
  }
  return c.json({ success: true, data: await saveSettings(patch) });
});
