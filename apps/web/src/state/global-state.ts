import { observable } from "@legendapp/state";

export interface GlobalState {
  activeProjectId: string | null;
  activeSessionId: string | null;
  isSidebarOpen: boolean;
}

export const globalState$ = observable<GlobalState>({
  activeProjectId: null,
  activeSessionId: null,
  isSidebarOpen: true,
});
