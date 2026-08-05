import { useQuery } from "@tanstack/react-query";
import { configService } from "../services/config.service";

export const configKeys = {
  approvalModes: ["config", "approval-modes"] as const,
};

export function useApprovalModes() {
  return useQuery({
    queryKey: configKeys.approvalModes,
    queryFn: () => configService.getApprovalModes(),
  });
}
