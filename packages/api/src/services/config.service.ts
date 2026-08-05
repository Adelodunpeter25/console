import { getConsoleApiClient } from "../client";
import type { ApprovalModeOption } from "@console/types";

function unwrapData<T>(body: { success?: boolean; data?: T; error?: string }, action: string): T {
  if (body?.success === false || body?.data === undefined) {
    throw new Error(body?.error || `Failed to ${action}`);
  }
  return body.data;
}

export const configService = {
  async getApprovalModes(): Promise<ApprovalModeOption[]> {
    const res = await getConsoleApiClient().get("/api/config/approval-modes");
    return unwrapData(res.data, "list approval modes");
  },
};
