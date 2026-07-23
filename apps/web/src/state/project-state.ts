import { observable } from "@legendapp/state";

export const activeProjectId$ = observable<string | null>(null);
