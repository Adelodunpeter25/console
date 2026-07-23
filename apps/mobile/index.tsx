import "./global.css";
import React, { useState, useEffect } from "react";
import { registerRootComponent } from "expo";
import { StatusBar } from "expo-status-bar";
import { View, Text, ActivityIndicator, TouchableOpacity } from "react-native";
import { SafeAreaProvider, SafeAreaView } from "react-native-safe-area-context";
import {
  useFonts,
  JetBrainsMono_400Regular,
  JetBrainsMono_500Medium,
  JetBrainsMono_600SemiBold,
  JetBrainsMono_700Bold,
} from "@expo-google-fonts/jetbrains-mono";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { ConsoleApiProvider, configureConsoleApi } from "@console/api";
import { QueryClient } from "@tanstack/react-query";
import { MainContent } from "./components/common/main-content";
import { BottomNav } from "./components/navigation/bottom-nav";

const queryClient = new QueryClient();
const BACKEND_URL_KEY = "@console_backend_url";

function AppRoot() {
  const [backendUrl, setBackendUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const [fontsLoaded] = useFonts({
    JetBrainsMono: JetBrainsMono_400Regular,
    "JetBrainsMono-Medium": JetBrainsMono_500Medium,
    "JetBrainsMono-SemiBold": JetBrainsMono_600SemiBold,
    "JetBrainsMono-Bold": JetBrainsMono_700Bold,
  });

  // Tab State: 'home' | 'chat' | 'settings'
  const [activeTab, setActiveTab] = useState<"home" | "chat" | "settings">("home");
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
        configureConsoleApi({ baseUrl: stored });
      } else {
        setActiveTab("settings");
      }
    } catch {
      setActiveTab("settings");
    } finally {
      setLoading(false);
    }
  };

  if (loading || !fontsLoaded) {
    return (
      <View className="flex-1 bg-[#0a0a0b] items-center justify-center">
        <ActivityIndicator size="large" color="#ffffff" />
      </View>
    );
  }

  return (
    <SafeAreaProvider>
      <View className="flex-1 bg-[#0a0a0b]">
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
              setBackendUrl={setBackendUrl}
            />
          </ConsoleApiProvider>
        ) : (
          <SafeAreaView className="flex-1 bg-[#0a0a0b] justify-center items-center p-6">
            <Text className="text-white text-lg font-bold mb-2">Welcome to Console Mobile</Text>
            <Text className="text-zinc-400 text-sm text-center mb-6 max-w-xs leading-6">
              Please specify your server endpoint in Settings to get started.
            </Text>
            <TouchableOpacity
              className="bg-white py-3 px-6 rounded-full"
              onPress={() => setActiveTab("settings")}
            >
              <Text className="text-sm font-bold text-black">Configure Server URL</Text>
            </TouchableOpacity>
          </SafeAreaView>
        )}

        <BottomNav
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          selectedSessionId={selectedSessionId}
        />
      </View>
    </SafeAreaProvider>
  );
}

registerRootComponent(AppRoot);
export default AppRoot;
