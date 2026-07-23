import "./global.css";
import React, { useState, useEffect } from "react";
import { registerRootComponent } from "expo";
import { StatusBar } from "expo-status-bar";
import { View, Text, ActivityIndicator, Alert } from "react-native";
import { SafeAreaProvider, SafeAreaView } from "react-native-safe-area-context";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { ConsoleApiProvider, configureConsoleApi } from "@console/api";
import { QueryClient } from "@tanstack/react-query";
import { MainContent } from "./components/common/main-content";
import { ConfigModal } from "./components/modal/config-modal";
import { BottomNav } from "./components/navigation/bottom-nav";

const queryClient = new QueryClient();
const BACKEND_URL_KEY = "@console_backend_url";

function AppRoot() {
  const [backendUrl, setBackendUrl] = useState<string | null>(null);
  const [inputUrl, setInputUrl] = useState("http://localhost:3000");
  const [showConfigModal, setShowConfigModal] = useState(false);
  const [loading, setLoading] = useState(true);

  // Tab State: 'home' | 'chat'
  const [activeTab, setActiveTab] = useState<"home" | "chat">("home");
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  useEffect(() => {
    loadBackendUrl();
  }, []);

  const loadBackendUrl = async () => {
    try {
      const stored = await AsyncStorage.getItem(BACKEND_URL_KEY);
      if (stored) {
        setBackendUrl(stored);
        setInputUrl(stored);
        configureConsoleApi({ baseUrl: stored });
      }
      setShowConfigModal(true);
    } catch {
      setShowConfigModal(true);
    } finally {
      setLoading(false);
    }
  };

  const saveBackendUrl = async () => {
    if (!inputUrl.trim()) {
      Alert.alert("Error", "Backend URL cannot be empty");
      return;
    }
    try {
      await AsyncStorage.setItem(BACKEND_URL_KEY, inputUrl.trim());
      setBackendUrl(inputUrl.trim());
      configureConsoleApi({ baseUrl: inputUrl.trim() });
      setShowConfigModal(false);
    } catch {
      Alert.alert("Error", "Failed to save URL");
    }
  };

  if (loading) {
    return (
      <View className="flex-1 bg-[#0d0d0e] items-center justify-center">
        <ActivityIndicator size="large" color="#ffffff" />
      </View>
    );
  }

  return (
    <SafeAreaProvider>
      <View className="flex-1 bg-[#0d0d0e]">
        <StatusBar style="light" />

        {backendUrl ? (
          <ConsoleApiProvider baseUrl={backendUrl} queryClient={queryClient}>
            <MainContent
              activeTab={activeTab}
              selectedProjectId={selectedProjectId}
              setSelectedProjectId={setSelectedProjectId}
              selectedSessionId={selectedSessionId}
              setSelectedSessionId={setSelectedSessionId}
              setActiveTab={setActiveTab}
              backendUrl={backendUrl}
            />
          </ConsoleApiProvider>
        ) : (
          <SafeAreaView className="flex-1 bg-[#0d0d0e]">
            <View className="flex-1 items-center justify-center p-6">
              <Text className="text-[#9095a0] text-center text-sm">
                Please configure the backend URL to start.
              </Text>
            </View>
          </SafeAreaView>
        )}

        <ConfigModal
          visible={showConfigModal}
          backendUrl={backendUrl}
          inputUrl={inputUrl}
          setInputUrl={setInputUrl}
          onSave={saveBackendUrl}
          onClose={() => {
            if (backendUrl) setShowConfigModal(false);
          }}
        />

        <BottomNav
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          selectedSessionId={selectedSessionId}
          onOpenConfig={() => setShowConfigModal(true)}
        />
      </View>
    </SafeAreaProvider>
  );
}

registerRootComponent(AppRoot);
export default AppRoot;
