import { batch, observable } from "@legendapp/state";
import { configureConsoleApi } from "@console/api";
import { appStorage } from "@/utils/storage";
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

/**
 * Backend environments (named server URLs) as Legend State observables.
 * Persisted to MMKV manually — see persist() below — because the storage
 * format also maintains the legacy single-URL key for downgrades.
 */
export const environments$ = observable({
  environments: [] as Environment[],
  activeId: null as string | null,
  probes: {} as Record<string, ProbeResult>,
});

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

// Seed from storage once at module load (mirrors the old zustand initial state).
{
  const persisted = loadPersisted();
  environments$.environments.set(persisted.environments);
  environments$.activeId.set(persisted.activeId);
}

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

export function addEnvironment(name: string, url: string): Environment {
  // Same normalized URL as an existing environment -> not a duplicate.
  const existing = environments$.environments.peek().find((e) => e.url === url);
  if (existing) {
    activateEnvironment(existing.id);
    return existing;
  }
  const currentEnvs = environments$.environments.peek();
  const currentActiveId = environments$.activeId.peek();
  const previous = currentEnvs.find((e) => e.id === currentActiveId);
  if (previous && previous.url !== url) {
    // The new environment replaces a different connected backend.
    resetServerState();
  }
  const env: Environment = { id: newId(), name: name.trim(), url };
  environments$.environments.push(env);
  environments$.activeId.set(env.id);
  persist([...currentEnvs, env], env.id, env.url);
  // A newly added environment becomes the connected backend.
  applyActive(env.url);
  return env;
}

export function updateEnvironment(id: string, patch: Partial<Pick<Environment, "name" | "url">>): void {
  const envs = environments$.environments.peek().map((e) =>
    e.id === id ? { ...e, ...patch } : e,
  );
  const isActive = environments$.activeId.peek() === id;
  const active = envs.find((e) => e.id === id) ?? null;
  environments$.environments.set(envs);
  persist(envs, environments$.activeId.peek(), isActive && active ? active.url : activeUrlOf({ environments: envs, activeId: environments$.activeId.peek() }));
  if (isActive && active) {
    applyActive(active.url);
  }
}

export function removeEnvironment(id: string): void {
  const envs = environments$.environments.peek().filter((e) => e.id !== id);
  const wasActive = environments$.activeId.peek() === id;
  const nextActiveId = wasActive ? null : environments$.activeId.peek();
  environments$.environments.set(envs);
  environments$.activeId.set(nextActiveId);
  environments$.probes[id].delete();
  persist(envs, nextActiveId, wasActive ? null : activeUrlOf({ environments: envs, activeId: nextActiveId }));
  if (nextActiveId === null) {
    // Active environment removed — drop server-scoped state.
    resetServerState();
    configureConsoleApi({ baseUrl: "" });
    setBackendUrl(null);
  }
}

/** Switch the active backend; clears server-scoped caches when the URL changes. */
export function activateEnvironment(id: string): void {
  const envs = environments$.environments.peek();
  const env = envs.find((e) => e.id === id);
  if (!env) return;
  const previous = envs.find((e) => e.id === environments$.activeId.peek());
  const unchanged = environments$.activeId.peek() === id || previous?.url === env.url;
  if (!unchanged && previous) {
    // Switching servers: stale messages/sessions/providers must not leak.
    resetServerState();
  }
  environments$.activeId.set(id);
  persist(envs, id, env.url);
  applyActive(env.url);
}

/** Full reset — removes every environment and all connection config, like a
 *  clean install. */
export function deactivate(): void {
  appStorage.remove(ENVIRONMENTS_KEY);
  appStorage.remove(BACKEND_URL_KEY);
  resetServerState();
  configureConsoleApi({ baseUrl: "" });
  setBackendUrl(null);
  batch(() => {
    environments$.environments.set([]);
    environments$.activeId.set(null);
    environments$.probes.set({});
  });
}

export async function probeEnvironment(id: string): Promise<boolean> {
  const env = environments$.environments.peek().find((e) => e.id === id);
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
  environments$.probes[id].set({ ok, checkedAt: Date.now() });
  return ok;
}
