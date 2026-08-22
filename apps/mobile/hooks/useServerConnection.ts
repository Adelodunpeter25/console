import { useState, useEffect, useCallback } from "react";
import { configureConsoleApi } from "@console/api";
import { confirmAlert } from "../components/common/confirm-dialog";
import { useAppStore } from "../stores/useAppStore";
import { useChatStore } from "../stores/useChatStore";
import { appStorage } from "../utils/storage";
import { queryClient } from "../query-client";

const BACKEND_URL_KEY = "@console_backend_url";

/** Clear the local chat cache (in-memory + persisted). Called when the
 *  backend URL changes so stale messages from a different server don't
 *  leak into the new connection. */
function clearChatCache() {
  try {
    useChatStore.setState({ sessions: {} });
    useChatStore.persist?.clearStorage?.();
  } catch (err) {
    console.warn("Could not clear persisted chat storage:", err);
  }
}

export function useServerConnection() {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const setBackendUrl = useAppStore((state) => state.setBackendUrl);
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const [inputUrl, setInputUrl] = useState("");
  const [loading, setLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [testingStatus, setTestingStatus] = useState<"idle" | "testing" | "success" | "error">("idle");

  const loadBackendUrl = useCallback(() => {
    try {
      const stored = appStorage.getString(BACKEND_URL_KEY);
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

  const saveConnection = useCallback(() => {
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
      // If switching to a different server, drop the local chat cache AND the
      // react-query cache so stale messages/sessions from the old server don't
      // leak into the new connection.
      if (backendUrl && backendUrl !== url) {
        clearChatCache();
        queryClient.clear();
      }
      appStorage.set(BACKEND_URL_KEY, url);
      configureConsoleApi({ baseUrl: url });
      setBackendUrl(url);
      setInputUrl(url);
      confirmAlert("Success", "Console backend server endpoint updated successfully!");
    } catch (err) {
      console.error("Failed to save endpoint URL:", err);
      confirmAlert(
        "Error",
        `Failed to save endpoint URL: ${err instanceof Error ? err.message : String(err)}`,
      );
    } finally {
      setIsSaving(false);
    }
  }, [inputUrl, backendUrl, setBackendUrl]);

  const testConnection = useCallback(async () => {
    if (!inputUrl.trim()) return;
    setTestingStatus("testing");
    try {
      let url = inputUrl.trim().replace(/\/+$/, "");
      if (!url.startsWith("http://") && !url.startsWith("https://")) {
        url = `http://${url}`;
      }
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 6000);
      const res = await fetch(`${url}/api/projects`, { signal: controller.signal });
      clearTimeout(timeoutId);
      setTestingStatus(res.ok ? "success" : "error");
    } catch {
      setTestingStatus("error");
    }
  }, [inputUrl]);

  const disconnect = useCallback(() => {
    confirmAlert(
      "Disconnect Backend",
      "Are you sure you want to clear the saved backend URL and reset connection data?",
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Disconnect",
          style: "destructive",
          onPress: () => {
            appStorage.remove(BACKEND_URL_KEY);
            clearChatCache();
            configureConsoleApi({ baseUrl: "" });
            queryClient.clear();
            setBackendUrl(null);
            setInputUrl("");
            setTestingStatus("idle");
            setActiveTab("home");
          },
        },
      ],
    );
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
