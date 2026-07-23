import { observable } from "@legendapp/state";

export const activeSessionId$ = observable<string | null>(null);
