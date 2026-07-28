import { useState, useEffect, useCallback } from "react";
import { Alert } from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { configureConsoleApi } from "@console/api";
import { useAppStore } from "../stores/useAppStore";

const BACKEND_URL_KEY = "@console_backend_url";

export function useServerConnection() {
  const backendUrl = useAppStore((state) => state.backendUrl);
  const setBackendUrl = useAppStore((state) => state.setBackendUrl);
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const [inputUrl, setInputUrl] = useState(backendUrl || "http://localhost:3000");
  const [loading, setLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [testingStatus, setTestingStatus] = useState<string | null>(null);

  const loadBackendUrl = useCallback(async () => {
    try {
      const stored = await AsyncStorage.getItem(BACKEND_URL_KEY);
      if (stored) {
        setBackendUrl(stored);
        setInputUrl(stored);
        configureConsoleApi({ baseUrl: stored });
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
      Alert.alert("Invalid URL", "Backend server endpoint cannot be empty.");
      return;
    }
    setIsSaving(true);
    try {
      const url = inputUrl.trim();
      await AsyncStorage.setItem(BACKEND_URL_KEY, url);
      configureConsoleApi({ baseUrl: url });
      setBackendUrl(url);
      Alert.alert("Success", "Console backend server endpoint updated successfully!");
    } catch {
      Alert.alert("Error", "Failed to save endpoint URL.");
    } finally {
      setIsSaving(false);
    }
  }, [inputUrl, setBackendUrl]);

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
    Alert.alert("Disconnect Backend", "Are you sure you want to clear the saved backend URL?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Disconnect",
        style: "destructive",
        onPress: async () => {
          await AsyncStorage.removeItem(BACKEND_URL_KEY);
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
