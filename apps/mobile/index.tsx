import "./global.css";
import React, { useEffect } from "react";
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
import { QueryClientProvider } from "@tanstack/react-query";
import { configureConsoleApi } from "@console/api";
import { KeyboardProvider } from "react-native-keyboard-controller";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { BottomSheetModalProvider } from "@gorhom/bottom-sheet";
import { MainContent } from "./components/layout/main-content";
import { ConfirmDialog } from "./components/common/confirm-dialog";
import { ErrorBoundary } from "./components/common/error-boundary";
import { useServerConnection, useOAuthDeepLink } from "./hooks";
import { queryClient } from "./query-client";

function OnboardingScreen() {
  const { inputUrl, setInputUrl, saveConnection, isSaving, testConnection, testingStatus } =
    useServerConnection();

  return (
    <SafeAreaView className="flex-1 bg-screen justify-center p-6">
      <Text className="text-foreground text-2xl font-bold mb-2 tracking-tight">Console Mobile</Text>
      <Text className="text-foreground-secondary text-sm mb-8 leading-6">
        Enter your Console server URL to connect.
      </Text>

      <TextInput
        value={inputUrl}
        onChangeText={setInputUrl}
        placeholder="http://192.168.1.X:3000"
        placeholderTextColor="#71717a"
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="url"
        className="bg-card border border-border rounded-xl px-4 py-3.5 text-foreground text-sm mb-4"
      />

      <TouchableOpacity
        onPress={() => saveConnection()}
        disabled={isSaving || !inputUrl.trim()}
        className={`bg-foreground rounded-xl py-3.5 items-center justify-center mb-3 ${
          isSaving || !inputUrl.trim() ? "opacity-50" : "active:opacity-80"
        }`}
      >
        {isSaving ? (
          <ActivityIndicator size="small" color="#000000" />
        ) : (
          <Text className="text-sm font-bold text-black">Connect</Text>
        )}
      </TouchableOpacity>

      <TouchableOpacity
        onPress={() => void testConnection()}
        disabled={!inputUrl.trim() || testingStatus === "testing"}
        className={`border border-border rounded-xl py-3.5 items-center justify-center ${
          !inputUrl.trim() || testingStatus === "testing" ? "opacity-50" : "active:opacity-80"
        }`}
      >
        {testingStatus === "testing" ? (
          <ActivityIndicator size="small" color="#a1a1aa" />
        ) : (
          <Text className="text-sm font-semibold text-foreground-secondary">
            {testingStatus === "success"
              ? "✓ Connection successful"
              : testingStatus === "error"
                ? "✕ Connection failed — try again"
                : "Test Connection"}
          </Text>
        )}
      </TouchableOpacity>
    </SafeAreaView>
  );
}

function AppRoot() {
  const { backendUrl, loading } = useServerConnection();
  useOAuthDeepLink();

  const [fontsLoaded] = useFonts({
    JetBrainsMono: JetBrainsMono_400Regular,
    "JetBrainsMono-Medium": JetBrainsMono_500Medium,
    "JetBrainsMono-SemiBold": JetBrainsMono_600SemiBold,
    "JetBrainsMono-Bold": JetBrainsMono_700Bold,
  });

  useEffect(() => {
    if (backendUrl) {
      configureConsoleApi({ baseUrl: backendUrl });
    }
  }, [backendUrl]);

  if (loading || !fontsLoaded) {
    return (
      <View className="flex-1 bg-screen items-center justify-center">
        <ActivityIndicator size="large" color="#ffffff" />
      </View>
    );
  }

  return (
    <QueryClientProvider client={queryClient}>
      <GestureHandlerRootView style={{ flex: 1 }}>
        <SafeAreaProvider>
          <View style={{ flex: 1, backgroundColor: "#0a0a0b" }}>
            <StatusBar style="light" />

            {backendUrl ? (
              <KeyboardProvider>
                <BottomSheetModalProvider>
                  <ErrorBoundary>
                    <MainContent />
                  </ErrorBoundary>
                </BottomSheetModalProvider>
              </KeyboardProvider>
            ) : (
              <ErrorBoundary>
                <OnboardingScreen />
              </ErrorBoundary>
            )}

            <ConfirmDialog />
          </View>
        </SafeAreaProvider>
      </GestureHandlerRootView>
    </QueryClientProvider>
  );
}

registerRootComponent(AppRoot);
export default AppRoot;
