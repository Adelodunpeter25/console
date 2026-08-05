import "./global.css";
import React from "react";
import { registerRootComponent } from "expo";
import { StatusBar } from "expo-status-bar";
import { View, Text, TextInput, TouchableOpacity, ActivityIndicator } from "react-native";
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
import { MainContent } from "./components/layout/main-content";
import { useServerConnection } from "./hooks";

const queryClient = new QueryClient();

function OnboardingScreen() {
  const { inputUrl, setInputUrl, saveConnection, isSaving } = useServerConnection();

  return (
    <SafeAreaView className="flex-1 bg-screen justify-center p-6">
      <Text className="text-foreground text-2xl font-bold mb-2 tracking-tight">Console Mobile</Text>
      <Text className="text-foreground-secondary text-sm mb-8 leading-6">
        Enter your Console backend server URL to get started.
      </Text>

      <TextInput
        className="h-12 bg-card border border-border rounded-xl px-4 text-foreground text-sm font-mono mb-4"
        value={inputUrl}
        onChangeText={setInputUrl}
        placeholder="http://192.168.1.X:3000"
        placeholderTextColor="#71717a"
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="url"
      />

      <TouchableOpacity
        className="bg-foreground py-3.5 px-6 rounded-full items-center"
        onPress={saveConnection}
        disabled={isSaving}
      >
        {isSaving ? (
          <ActivityIndicator size="small" color="#000000" />
        ) : (
          <Text className="text-sm font-bold text-black">Connect</Text>
        )}
      </TouchableOpacity>
    </SafeAreaView>
  );
}

function AppRoot() {
  const { backendUrl, loading } = useServerConnection();

  const [fontsLoaded] = useFonts({
    JetBrainsMono: JetBrainsMono_400Regular,
    "JetBrainsMono-Medium": JetBrainsMono_500Medium,
    "JetBrainsMono-SemiBold": JetBrainsMono_600SemiBold,
    "JetBrainsMono-Bold": JetBrainsMono_700Bold,
  });

  if (loading || !fontsLoaded) {
    return (
      <View className="flex-1 bg-screen items-center justify-center">
        <ActivityIndicator size="large" color="#ffffff" />
      </View>
    );
  }

  return (
    <SafeAreaProvider>
      <View className="flex-1 bg-screen" style={{ flex: 1 }}>
        <StatusBar style="light" />

        {backendUrl ? (
          <ConsoleApiProvider baseUrl={backendUrl} queryClient={queryClient}>
            <MainContent />
          </ConsoleApiProvider>
        ) : (
          <OnboardingScreen />
        )}
      </View>
    </SafeAreaProvider>
  );
}

registerRootComponent(AppRoot);
export default AppRoot;
