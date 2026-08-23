import { useState, useEffect, useCallback } from "react";
import { configureConsoleApi } from "@console/api";
import { confirmAlert } from "@/components/common/confirm-dialog";
import { useAppStore } from "@/stores/useAppStore";
import {
  useEnvironmentsStore,
  type Environment,
} from "@/stores/useEnvironmentsStore";
import { normalizeBackendUrl } from "@/utils/url";
import { resetServerState } from "@/utils/server-state";

/**
 * Legacy single-endpoint connection API, kept intact for the onboarding
 * screen and root app shell. Storage and switching now live in
 * `useEnvironmentsStore` — this hook delegates to it so both entry points
 * stay in sync with the environments feature.
 */
export function useServerConnection() {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const environments = useEnvironmentsStore((state) => state.environments);
  const activeId = useEnvironmentsStore((state) => state.activeId);

  const [inputUrl, setInputUrl] = useState("");
  const [loading, setLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [testingStatus, setTestingStatus] = useState<"idle" | "testing" | "success" | "error">("idle");

  const activeEnv: Environment | null =
    environments.find((env) => env.id === activeId) ?? null;

  // Apply the persisted active environment once on mount.
  useEffect(() => {
    if (activeEnv) {
      configureConsoleApi({ baseUrl: activeEnv.url });
      useAppStore.getState().setBackendUrl(activeEnv.url);
    }
    setLoading(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (activeEnv) {
      setInputUrl(activeEnv.url);
    }
  }, [activeEnv?.id]);

  const saveConnection = useCallback(
    (name?: string) => {
      const url = normalizeBackendUrl(inputUrl);
      if (!url) {
        confirmAlert("Invalid URL", "Backend server endpoint cannot be empty.");
        return;
      }
      setIsSaving(true);
      try {
        const store = useEnvironmentsStore.getState();
        if (store.activeId && store.environments.some((e) => e.id === store.activeId)) {
          // Editing the current environment's URL in place.
          const previous = store.environments.find((e) => e.id === store.activeId);
          if (previous && previous.url !== url) {
            resetServerState();
          }
          store.updateEnvironment(store.activeId, { url });
        } else {
          store.addEnvironment(name?.trim() || "Default", url);
        }
        setInputUrl(url);
      } catch (err) {
        console.error("Failed to save endpoint URL:", err);
        confirmAlert(
          "Error",
          `Failed to save endpoint URL: ${err instanceof Error ? err.message : String(err)}`,
        );
      } finally {
        setIsSaving(false);
      }
    },
    [inputUrl],
  );

  const testConnection = useCallback(async () => {
    const url = normalizeBackendUrl(inputUrl);
    if (!url) return;
    setTestingStatus("testing");
    try {
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
      "Are you sure you want to disconnect? This removes all environments and connection data, like a clean install.",
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Disconnect",
          style: "destructive",
          onPress: () => {
            useEnvironmentsStore.getState().deactivate();
            setInputUrl("");
            setTestingStatus("idle");
            setActiveTab("home");
          },
        },
      ],
    );
  }, [setActiveTab]);

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
