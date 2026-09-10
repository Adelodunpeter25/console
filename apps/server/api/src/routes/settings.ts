import { Hono } from "hono";
import { isConsoleModelRole, loadSettings, saveSettings, type ModelRoleMapping } from "@/agent/src/service/model-roles.js";

export const settingsRoutes = new Hono();

settingsRoutes.get("/settings", async (c) => c.json({ success: true, data: await loadSettings() }));

settingsRoutes.patch("/settings", async (c) => {
  const body = await c.req.json<unknown>();
  if (!body || typeof body !== "object" || !Object.hasOwn(body, "modelRoles")) {
    return c.json({ success: false, error: "modelRoles is required." }, 400);
  }
  const raw = (body as { modelRoles?: unknown }).modelRoles;
  if (!raw || typeof raw !== "object") return c.json({ success: false, error: "modelRoles must be an object." }, 400);
  const modelRoles: ModelRoleMapping = {};
  for (const [role, model] of Object.entries(raw)) {
    if (!isConsoleModelRole(role) || typeof model !== "string" || !model.trim()) {
      return c.json({ success: false, error: `Invalid model role '${role}'.` }, 400);
    }
    modelRoles[role] = model.trim();
  }
  return c.json({ success: true, data: await saveSettings({ modelRoles }) });
});
