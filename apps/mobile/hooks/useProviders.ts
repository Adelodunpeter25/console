import { useProviders, useApprovalModes } from "@console/api";

/** Provider + approval-mode catalog for the settings UI. */
export function useProviderCatalog() {
  const { data: providers = [], isLoading: loadingProviders } = useProviders();
  const { data: approvalModes = [], isLoading: loadingApprovalModes } = useApprovalModes();

  return {
    providers,
    approvalModes,
    loadingProviders,
    loadingApprovalModes,
  };
}
