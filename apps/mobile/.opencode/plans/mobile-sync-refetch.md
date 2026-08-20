# Mobile — Refetch on Focus + Interval (QueryClient) — Implementation Plan

**Context:** `apps/mobile/query-client.ts:8` and `packages/api/src/query-client.ts:6` both use `staleTime: 5min, refetchOnWindowFocus:false`. Counterpart to desktop `.opencode/plans/tier1-sync-refresh.md` which polls every 15s. Mobile currently needs `hooks/useHomeSessions.ts:122 onRefresh` pull-to-refresh to see desktop-created chats — same staleness opposite direction.

**Goal:** Foregrounding app or 15s interval refreshes lists so cross-device creates appear without restart. No backend change. Sym symmetric to desktop Tier 1.

---

## 1. Files

* `apps/mobile/query-client.ts` — add RN `AppState` → `focusManager` bridge
* `packages/api/src/query-client.ts` — same bridge (shared)
* `packages/api/src/hooks/useSessions.ts` — override `useSessions`/`useSession` stale/interval
* `packages/api/src/hooks/useProjects.ts` (if exists) or `apps/mobile/hooks/useProjects.ts:1` similarly
* `apps/mobile/hooks/useHomeSessions.ts:18` — keep `onRefresh` but also rely on auto

---

## 2. Patch

### 2.1 `apps/mobile/query-client.ts`

```ts
import { QueryClient, focusManager } from "@tanstack/react-query";
import { AppState } from "react-native";

focusManager.setEventListener(setFocused => {
  const sub = AppState.addEventListener("change", s => setFocused(s === "active"));
  return () => sub.remove();
});

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 5, // keep global for detail/cached
      retry: 2,
      refetchOnWindowFocus: false, // enable per-list query instead of global storm
    },
  },
});
```

### 2.2 `packages/api/src/query-client.ts`

Same AppState guard — only run when `AppState` exists (web keeps native `focusManager`):

```ts
import { focusManager } from "@tanstack/react-query";
try {
  const { AppState } = require("react-native");
  focusManager.setEventListener(setFocused => {
    const sub = AppState.addEventListener("change", s => setFocused(s === "active"));
    return () => sub.remove();
  });
} catch {}
```

### 2.3 `packages/api/src/hooks/useSessions.ts`

```ts
export function useSessions(params?) {
  return useQuery({
    queryKey: sessionKeys.lists(params),
    queryFn: () => sessionService.getSessions(params),
    staleTime: 15_000,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
  });
}
export function useSession(id: string) {
  return useQuery({
    queryKey: sessionKeys.detail(id),
    queryFn: () => sessionService.getSession(id),
    enabled: Boolean(id),
    staleTime: 15_000,
    refetchOnWindowFocus: true,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
  });
}
```

Do same for `useProjects`/`useProjects` hook (hook already does `refetch: loadProjects` but Query-based one should get interval).

---

## 3. Verification

* Start desktop create chat → background mobile 16s → foreground → list shows new chat without pull.
* Desktop create message in open session → mobile foreground detects same session `useSession` refetch → transcript updates.
* Battery: `refetchIntervalInBackground:false` ensures no poll when backgrounded.

---

## 4. Not in Scope

* Push SSE (`/watch`) — Tier 3
* Offline queue — existing `invalidateQueries` on mutations already covers own writes

---

## Relation to Desktop

Mirrors `gpui-ui/.opencode/plans/tier1-sync-refresh.md:3` desktop poll 15s + `observe_window_activation`. Both sides eventual ~15s.
