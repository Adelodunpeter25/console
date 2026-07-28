import { create } from "zustand";
import type { ApprovalModeOption, Model, ProviderCatalogEntry } from "@console/types";
import { tauriApi } from "../lib/tauri-api";

interface ProviderState {
  /** Full provider catalog (name, display name, description, static models). */
  providers: ProviderCatalogEntry[];
  /** Models fetched dynamically per provider, keyed by provider id. */
  modelsByProvider: Record<string, Model[]>;
  loadingProviders: boolean;
  /** Set of provider ids currently fetching models. */
  loadingModels: Record<string, boolean>;
  error: string | null;

  /** Approval mode options fetched from the backend. */
  approvalModes: ApprovalModeOption[];
  loadingApprovalModes: boolean;

  loadProviders: () => Promise<void>;
  loadModels: (providerId: string) => Promise<Model[]>;
  loadApprovalModes: () => Promise<void>;
  clearError: () => void;
}

export const useProviderStore = create<ProviderState>((set, get) => ({
  providers: [],
  modelsByProvider: {},
  loadingProviders: false,
  loadingModels: {},
  error: null,
  approvalModes: [],
  loadingApprovalModes: false,

  loadProviders: async () => {
    set({ loadingProviders: true, error: null });
    try {
      const providers = await tauriApi.listProviders();
      set({ providers, loadingProviders: false });
    } catch (e) {
      set({
        loadingProviders: false,
        error: e instanceof Error ? e.message : "Failed to load providers",
      });
    }
  },

  loadModels: async (providerId: string) => {
    // Return cached models when available.
    const cached = get().modelsByProvider[providerId];
    if (cached) return cached;

    set((s) => ({
      loadingModels: { ...s.loadingModels, [providerId]: true },
      error: null,
    }));
    try {
      const result = await tauriApi.getProviderModels(providerId);
      const models = result.models;
      set((s) => ({
        modelsByProvider: { ...s.modelsByProvider, [providerId]: models },
        loadingModels: { ...s.loadingModels, [providerId]: false },
      }));
      return models;
    } catch (e) {
      set((s) => ({
        loadingModels: { ...s.loadingModels, [providerId]: false },
        error: e instanceof Error ? e.message : "Failed to load models",
      }));
      throw e;
    }
  },

  loadApprovalModes: async () => {
    if (get().approvalModes.length > 0) return;
    set({ loadingApprovalModes: true, error: null });
    try {
      const modes = await tauriApi.getApprovalModes();
      set({ approvalModes: modes, loadingApprovalModes: false });
    } catch (e) {
      set({
        loadingApprovalModes: false,
        error: e instanceof Error ? e.message : "Failed to load approval modes",
      });
    }
  },

  clearError: () => set({ error: null }),
}));
