/**
 * Config Routes (/api/config/*).
 * Serves static configuration metadata that the desktop UI renders dynamically.
 */
import { Hono } from "hono";
import type { ApprovalModeOption } from "@console/types";

export const configRoutes = new Hono();

const APPROVAL_MODES: ApprovalModeOption[] = [
  {
    value: "always-ask",
    label: "Normal",
    description: "Ask for every action",
  },
  {
    value: "accept-edits",
    label: "Accept Edits",
    description: "Auto-approve file edits",
  },
  {
    value: "plan-mode",
    label: "Plan Mode",
    description: "Plan only, no execution",
  },
  {
    value: "full-access",
    label: "Bypass Permissions",
    description: "Run everything without asking",
  },
];

/**
 * GET /api/config/approval-modes — List all approval modes with display metadata.
 */
configRoutes.get("/config/approval-modes", (c) => {
  return c.json({
    success: true,
    data: APPROVAL_MODES,
  });
});
