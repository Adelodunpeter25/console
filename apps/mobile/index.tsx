import "./global.css";
import React from "react";
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
import { ConsoleApiProvider } from "@console/api";
import { QueryClient } from "@tanstack/react-query";
import { MainContent } from "./components/common/main-content";
import { BottomNav } from "./components/navigation/bottom-nav";
import { useServerConnection } from "./hooks";
import { useAppStore } from "./stores";

const queryClient = new QueryClient();

function AppRoot() {
  const { backendUrl, loading } = useServerConnection();
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const [fontsLoaded] = useFonts({
    JetBrainsMono: JetBrainsMono_400Regular,
    "JetBrainsMono-Medium": JetBrainsMono_500Medium,
    "JetBrainsMono-SemiBold": JetBrainsMono_600SemiBold,
    "JetBrainsMono-Bold": JetBrainsMono_700Bold,
  });

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
            <MainContent />
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

        <BottomNav />
      </View>
    </SafeAreaProvider>
  );
}

registerRootComponent(AppRoot);
export default AppRoot;
