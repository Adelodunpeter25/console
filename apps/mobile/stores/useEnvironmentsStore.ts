import { create } from "zustand";
import { configureConsoleApi } from "@console/api";
import { appStorage } from "@/utils/storage";
import { normalizeBackendUrl } from "@/utils/url";
import { resetServerState } from "@/utils/server-state";
import { queryClient } from "@/query-client";
import { loadAuthStatus } from "@/stores/useAuthStore";
import { setBackendUrl } from "./useAppStore";

/** Legacy single-URL key kept in sync so downgrades / other readers don't break. */
const BACKEND_URL_KEY = "@console_backend_url";
const ENVIRONMENTS_KEY = "@console_environments";

export interface Environment {
  /** `env_` + 4 random hex chars, assigned at creation. */
  id: string;
  name: string;
  url: string;
}

export interface ProbeResult {
  ok: boolean;
  checkedAt: number;
}

interface EnvironmentsState {
  environments: Environment[];
  activeId: string | null;
  probes: Record<string, ProbeResult>;
  addEnvironment: (name: string, url: string) => Environment;
  updateEnvironment: (id: string, patch: Partial<Pick<Environment, "name" | "url">>) => void;
  removeEnvironment: (id: string) => void;
  /** Switch the active backend; clears server-scoped caches when the URL changes. */
  activateEnvironment: (id: string) => void;
  /** Deactivate without deleting (disconnect flow). */
  deactivate: () => void;
  probeEnvironment: (id: string) => Promise<boolean>;
}

function newId(): string {
  return `env_${Math.random().toString(16).slice(2, 6)}`;
}

function persist(environments: Environment[], activeId: string | null, activeUrl: string | null) {
  appStorage.set(ENVIRONMENTS_KEY, JSON.stringify({ environments, activeId }));
  if (activeUrl) {
    appStorage.set(BACKEND_URL_KEY, activeUrl);
  } else {
    appStorage.remove(BACKEND_URL_KEY);
  }
}

function loadPersisted(): { environments: Environment[]; activeId: string | null } {
  try {
    const raw = appStorage.getString(ENVIRONMENTS_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as { environments?: Environment[]; activeId?: string | null };
      const environments = Array.isArray(parsed.environments) ? parsed.environments : [];
      return {
        environments,
        activeId:
          parsed.activeId && environments.some((e) => e.id === parsed.activeId)
            ? parsed.activeId
            : null,
      };
    }
  } catch (err) {
    console.warn("Could not read stored environments:", err);
  }
  // Migration: seed one "Default" environment from the legacy key.
  const legacy = appStorage.getString(BACKEND_URL_KEY);
  if (legacy) {
    const env: Environment = { id: newId(), name: "Default", url: legacy };
    appStorage.set(ENVIRONMENTS_KEY, JSON.stringify({ environments: [env], activeId: env.id }));
    return { environments: [env], activeId: env.id };
  }
  return { environments: [], activeId: null };
}

export const useEnvironmentsStore = create<EnvironmentsState>((set, get) => ({
  ...loadPersisted(),
  probes: {},

  addEnvironment: (name, url) => {
    const state = get();
    // Same normalized URL as an existing environment -> not a duplicate.
    const existing = state.environments.find((e) => e.url === url);
    if (existing) {
      get().activateEnvironment(existing.id);
      return existing;
    }
    const previous = state.environments.find((e) => e.id === state.activeId);
    if (previous && previous.url !== url) {
      // The new environment replaces a different connected backend.
      resetServerState();
    }
    const env: Environment = { id: newId(), name: name.trim(), url };
    set((current) => {
      persist([...current.environments, env], env.id, env.url);
      return { environments: [...current.environments, env], activeId: env.id };
    });
    // A newly added environment becomes the connected backend.
    applyActive(env.url);
    return env;
  },

  updateEnvironment: (id, patch) => {
    set((state) => {
      const environments = state.environments.map((e) =>
        e.id === id ? { ...e, ...patch } : e,
      );
      const isActive = state.activeId === id;
      const active = environments.find((e) => e.id === id) ?? null;
      persist(environments, state.activeId, isActive && active ? active.url : activeUrlOf(state));
      return { environments };
    });
    const state = get();
    const updated = state.environments.find((e) => e.id === id);
    if (state.activeId === id && updated) {
      applyActive(updated.url);
    }
  },

  removeEnvironment: (id) => {
    set((state) => {
      const environments = state.environments.filter((e) => e.id !== id);
      const wasActive = state.activeId === id;
      const activeId = wasActive ? null : state.activeId;
      persist(environments, activeId, wasActive ? null : activeUrlOf(state));
      const probes = { ...state.probes };
      delete probes[id];
      return { environments, activeId, probes };
    });
    if (get().activeId === null) {
      // Active environment removed — drop server-scoped state.
      resetServerState();
      configureConsoleApi({ baseUrl: "" });
      setBackendUrl(null);
    }
  },

  activateEnvironment: (id) => {
    const state = get();
    const env = state.environments.find((e) => e.id === id);
    if (!env) return;
    const previous = state.environments.find((e) => e.id === state.activeId);
    const unchanged = state.activeId === id || previous?.url === env.url;
    if (!unchanged && previous) {
      // Switching servers: stale messages/sessions/providers must not leak.
      resetServerState();
    }
    set(() => {
      persist(state.environments, id, env.url);
      return { activeId: id };
    });
    applyActive(env.url);
  },

  /** Full reset — removes every environment and all connection config, like a
   *  clean install. */
  deactivate: () => {
    appStorage.remove(ENVIRONMENTS_KEY);
    appStorage.remove(BACKEND_URL_KEY);
    resetServerState();
    configureConsoleApi({ baseUrl: "" });
    setBackendUrl(null);
    set({ environments: [], activeId: null, probes: {} });
  },

  probeEnvironment: async (id) => {
    const env = get().environments.find((e) => e.id === id);
    let ok = false;
    if (env) {
      try {
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), 6000);
        const res = await fetch(`${env.url}/api/projects`, { signal: controller.signal });
        clearTimeout(timeoutId);
        ok = res.ok;
      } catch {
        ok = false;
      }
    }
    set((state) => ({ probes: { ...state.probes, [id]: { ok, checkedAt: Date.now() } } }));
    return ok;
  },
}));

function activeUrlOf(state: { environments: Environment[]; activeId: string | null }): string | null {
  return state.environments.find((e) => e.id === state.activeId)?.url ?? null;
}

/** Push the active URL into the API client, app store and auth status. */
function applyActive(url: string) {
  configureConsoleApi({ baseUrl: url });
  setBackendUrl(url);
  void loadAuthStatus();
  queryClient.resumePausedMutations?.();
}
