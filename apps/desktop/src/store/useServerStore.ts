import { create } from "zustand";
import { api } from "../lib/api";

interface ServerState {
  backendUrl: string;
  connected: boolean;
  loading: boolean;
  testing: "idle" | "testing" | "success" | "error";
  init: () => Promise<void>;
  setUrl: (url: string) => Promise<void>;
  testConnection: () => Promise<void>;
  setConnected: (connected: boolean) => void;
}

export const useServerStore = create<ServerState>((set) => ({
  backendUrl: "http://localhost:3000",
  connected: false,
  loading: true,
  testing: "idle",

  init: async () => {
    try {
      const url = await api.getBackendUrl();
      set({ backendUrl: url, loading: false });
    } catch {
      set({ loading: false });
    }
  },

  setUrl: async (url: string) => {
    const trimmed = url.trim().replace(/\/+$/, "");
    await api.setBackendUrl(trimmed);
    set({ backendUrl: trimmed });
  },

  testConnection: async () => {
    set({ testing: "testing" });
    try {
      await api.pingServer();
      set({ testing: "success", connected: true });
    } catch {
      set({ testing: "error", connected: false });
    }
  },

  setConnected: (connected) => set({ connected }),
}));
