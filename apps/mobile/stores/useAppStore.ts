import { batch, observable } from "@legendapp/state";

export type MobileTab = "home" | "chat" | "settings" | "terminal" | "files";

/**
 * Global app UI state (active tab, selected project/session, backend URL) as
 * Legend State observables. See docs/legend-state-and-list-migration.md.
 *
 * Reads in components subscribe narrowly via `useValue(app$.field)`;
 * imperative reads outside render use `app$.field.peek()`.
 */
export const app$ = observable({
  activeTab: "home" as MobileTab,
  previousTab: null as MobileTab | null,
  selectedProjectId: null as string | null,
  selectedSessionId: null as string | null,
  backendUrl: null as string | null,
  pendingConnectionSection: false,
});

export function setActiveTab(tab: MobileTab): void {
  const current = app$.activeTab.peek();
  if (current === tab) return;
  batch(() => {
    app$.previousTab.set(current);
    app$.activeTab.set(tab);
  });
}

export function setSelectedProjectId(id: string | null): void {
  app$.selectedProjectId.set(id);
}

export function setSelectedSessionId(id: string | null): void {
  app$.selectedSessionId.set(id);
}

export function setBackendUrl(url: string | null): void {
  app$.backendUrl.set(url);
}

export function setPendingConnectionSection(pending: boolean): void {
  app$.pendingConnectionSection.set(pending);
}

export function clearAppSelections(): void {
  batch(() => {
    app$.selectedProjectId.set(null);
    app$.selectedSessionId.set(null);
  });
}
