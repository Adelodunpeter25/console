import { useEffect } from "react";
import { useValue } from "@legendapp/state/react";
import { loadApprovalModes, loadModels, loadProviders, provider$ } from "@/stores/useProviderStore";

/** Provider + approval-mode catalog backed by `provider$`. */
export function useProviderCatalog() {
  const providers = useValue(provider$.providers);
  const loadingProviders = useValue(provider$.loadingProviders);
  const approvalModes = useValue(provider$.approvalModes);
  const loadingApprovalModes = useValue(provider$.loadingApprovalModes);

  useEffect(() => {
    if (providers.length === 0 && !loadingProviders) {
      loadProviders().catch(() => {});
    }
    if (approvalModes.length === 0 && !loadingApprovalModes) {
      loadApprovalModes().catch(() => {});
    }
  }, [providers.length, loadingProviders, approvalModes.length, loadingApprovalModes]);

  return {
    providers,
    approvalModes,
    loadingProviders,
    loadingApprovalModes,
  };
}

/** Per-provider dynamic model list backed by `provider$` (cached). */
export function useProviderModels(providerId: string) {
  const models = useValue(() => provider$.modelsByProvider[providerId].get());
  const loadingModels = useValue(() => Boolean(provider$.loadingModels[providerId].get()));

  useEffect(() => {
    if (providerId && !models && !loadingModels) {
      loadModels(providerId).catch(() => {});
    }
  }, [providerId, models, loadingModels]);

  return {
    data: models ? { provider: providerId, models } : undefined,
    isLoading: loadingModels,
  };
}
