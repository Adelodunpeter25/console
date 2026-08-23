import { useEffect } from "react";
import { useProviderStore } from "@/stores/useProviderStore";

/** Provider + approval-mode catalog backed by `useProviderStore`. */
export function useProviderCatalog() {
  const providers = useProviderStore((state) => state.providers);
  const loadingProviders = useProviderStore((state) => state.loadingProviders);
  const approvalModes = useProviderStore((state) => state.approvalModes);
  const loadingApprovalModes = useProviderStore((state) => state.loadingApprovalModes);
  const loadProviders = useProviderStore((state) => state.loadProviders);
  const loadApprovalModes = useProviderStore((state) => state.loadApprovalModes);

  useEffect(() => {
    if (providers.length === 0 && !loadingProviders) {
      loadProviders().catch(() => {});
    }
    if (approvalModes.length === 0 && !loadingApprovalModes) {
      loadApprovalModes().catch(() => {});
    }
  }, [providers.length, loadingProviders, approvalModes.length, loadingApprovalModes, loadProviders, loadApprovalModes]);

  return {
    providers,
    approvalModes,
    loadingProviders,
    loadingApprovalModes,
  };
}

/** Per-provider dynamic model list backed by `useProviderStore` (cached). */
export function useProviderModels(providerId: string) {
  const models = useProviderStore((state) => state.modelsByProvider[providerId]);
  const loadingModels = useProviderStore((state) => Boolean(state.loadingModels[providerId]));
  const loadModels = useProviderStore((state) => state.loadModels);

  useEffect(() => {
    if (providerId && !models && !loadingModels) {
      loadModels(providerId).catch(() => {});
    }
  }, [providerId, models, loadingModels, loadModels]);

  return {
    data: models ? { provider: providerId, models } : undefined,
    isLoading: loadingModels,
  };
}
