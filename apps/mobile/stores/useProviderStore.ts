import { batch, observable } from "@legendapp/state";
import type { ApprovalModeOption, Model, ProviderCatalogEntry } from "@console/types";
import { providerService, configService } from "@console/api";

/**
 * Provider catalog + per-provider model cache as Legend State observables.
 * See docs/legend-state-and-list-migration.md.
 *
 * Reads in components subscribe via `useValue(provider$.field)`;
 * imperative reads outside render use `.peek()`.
 */
export const provider$ = observable({
  /** Full provider catalog (name, display name, description, static models). */
  providers: [] as ProviderCatalogEntry[],
  /** Models fetched dynamically per provider, keyed by provider id. */
  modelsByProvider: {} as Record<string, Model[]>,
  loadingProviders: false,
  /** Set of provider ids currently fetching models. */
  loadingModels: {} as Record<string, boolean>,
  error: null as string | null,
  /** Approval mode options fetched from the backend. */
  approvalModes: [] as ApprovalModeOption[],
  loadingApprovalModes: false,
});

export async function loadProviders(): Promise<void> {
  batch(() => {
    provider$.loadingProviders.set(true);
    provider$.error.set(null);
  });
  try {
    const providers = await providerService.getProviders();
    batch(() => {
      provider$.providers.set(providers);
      provider$.loadingProviders.set(false);
    });
  } catch (e) {
    batch(() => {
      provider$.loadingProviders.set(false);
      provider$.error.set(e instanceof Error ? e.message : "Failed to load providers");
    });
  }
}

export async function loadModels(providerId: string): Promise<Model[]> {
  // Return cached models when available.
  const cached = provider$.modelsByProvider[providerId].peek();
  if (cached) return cached;

  batch(() => {
    provider$.loadingModels[providerId].set(true);
    provider$.error.set(null);
  });
  try {
    const result = await providerService.getProviderModels(providerId);
    const models = result.models;
    batch(() => {
      provider$.modelsByProvider[providerId].set(models);
      provider$.loadingModels[providerId].set(false);
    });
    return models;
  } catch (e) {
    provider$.loadingModels[providerId].set(false);
    provider$.error.set(e instanceof Error ? e.message : "Failed to load models");
    throw e;
  }
}

export async function loadApprovalModes(): Promise<void> {
  if (provider$.approvalModes.peek().length > 0) return;
  batch(() => {
    provider$.loadingApprovalModes.set(true);
    provider$.error.set(null);
  });
  try {
    const modes = await configService.getApprovalModes();
    batch(() => {
      provider$.approvalModes.set(modes);
      provider$.loadingApprovalModes.set(false);
    });
  } catch (e) {
    batch(() => {
      provider$.loadingApprovalModes.set(false);
      provider$.error.set(e instanceof Error ? e.message : "Failed to load approval modes");
    });
  }
}

export function clearProviderError(): void {
  provider$.error.set(null);
}

export function clearProviderState(): void {
  batch(() => {
    provider$.providers.set([]);
    provider$.modelsByProvider.set({} as Record<string, Model[]>);
    provider$.loadingProviders.set(false);
    provider$.loadingModels.set({} as Record<string, boolean>);
    provider$.approvalModes.set([]);
    provider$.loadingApprovalModes.set(false);
    provider$.error.set(null);
  });
}
