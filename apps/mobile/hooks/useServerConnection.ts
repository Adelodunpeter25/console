import { useState, useEffect, useCallback } from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { configureConsoleApi } from "@console/api";
import { confirmAlert } from "../components/common/confirm-dialog";
import { useAppStore } from "../stores/useAppStore";
import { useChatStore } from "../stores/useChatStore";

const BACKEND_URL_KEY = "@console_backend_url";

/** Clear the local chat cache (in-memory + persisted). Called when the
 *  backend URL changes so stale messages from a different server don't
 *  leak into the new connection. */
function clearChatCache() {
  useChatStore.setState({ sessions: {} });
  void useChatStore.persist.clearStorage();
}

export function useServerConnection() {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const setBackendUrl = useAppStore((state) => state.setBackendUrl);
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const [inputUrl, setInputUrl] = useState("");
  const [loading, setLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [testingStatus, setTestingStatus] = useState<"idle" | "testing" | "success" | "error">("idle");

  const loadBackendUrl = useCallback(async () => {
    try {
      const stored = await AsyncStorage.getItem(BACKEND_URL_KEY);
      if (stored) {
        configureConsoleApi({ baseUrl: stored });
        setBackendUrl(stored);
        setInputUrl(stored);
      }
    } catch {
      // No URL stored — onboarding screen will show
    } finally {
      setLoading(false);
    }
  }, [setBackendUrl]);

  useEffect(() => {
    loadBackendUrl();
  }, [loadBackendUrl]);

  const saveConnection = useCallback(async () => {
    if (!inputUrl.trim()) {
      confirmAlert("Invalid URL", "Backend server endpoint cannot be empty.");
      return;
    }
    setIsSaving(true);
    try {
      let url = inputUrl.trim().replace(/\/+$/, "");
      if (!url.startsWith("http://") && !url.startsWith("https://")) {
        url = `http://${url}`;
      }
      // If switching to a different server, drop the local chat cache so
      // stale messages from the old server don't render against the new one.
      if (backendUrl && backendUrl !== url) {
        clearChatCache();
      }
      await AsyncStorage.setItem(BACKEND_URL_KEY, url);
      configureConsoleApi({ baseUrl: url });
      setBackendUrl(url);
      confirmAlert("Success", "Console backend server endpoint updated successfully!");
    } catch {
      confirmAlert("Error", "Failed to save endpoint URL.");
    } finally {
      setIsSaving(false);
    }
  }, [inputUrl, backendUrl, setBackendUrl]);

  const testConnection = useCallback(async () => {
    if (!inputUrl.trim()) return;
    setTestingStatus("testing");
    try {
      const res = await fetch(`${inputUrl.trim()}/api/projects`);
      setTestingStatus(res.ok ? "success" : "error");
    } catch {
      setTestingStatus("error");
    }
  }, [inputUrl]);

  const disconnect = useCallback(async () => {
    confirmAlert("Disconnect Backend", "Are you sure you want to clear the saved backend URL?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Disconnect",
        style: "destructive",
        onPress: async () => {
          await AsyncStorage.removeItem(BACKEND_URL_KEY);
          clearChatCache();
          setBackendUrl(null);
          setActiveTab("home");
        },
      },
    ]);
  }, [setActiveTab, setBackendUrl]);

  return {
    backendUrl,
    inputUrl,
    setInputUrl,
    loading,
    isSaving,
    testingStatus,
    saveConnection,
    testConnection,
    disconnect,
  };
}
