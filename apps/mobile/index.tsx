import "./global.css";
import { initNitroFetch } from "./utils/nitro-fetch";
initNitroFetch();

import React, { useEffect, useState } from "react";
import { registerRootComponent } from "expo";
import { StatusBar } from "expo-status-bar";
import { View, Text, TextInput, TouchableOpacity, ActivityIndicator } from "react-native";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";
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
import { theme } from "./styles/theme";

function OnboardingScreen() {
  const { inputUrl, setInputUrl, saveConnection, isSaving, testConnection, testingStatus } =
    useServerConnection();
  const [envName, setEnvName] = useState("");

  const insets = useSafeAreaInsets();

  return (
    // Inline styles here deliberately: className-based layout on
    // SafeAreaView proved unreliable (collapses to content height, content
    // slides under the status bar). Children keep their classNames.
    <View
      style={{
        flex: 1,
        backgroundColor: theme.colors.background,
        justifyContent: "center",
        paddingHorizontal: 32,
        paddingTop: insets.top,
      }}
    >
      <Text className="text-foreground text-3xl font-bold mb-2 tracking-tight text-center">
        Console Mobile
      </Text>
      <Text className="text-foreground-secondary text-sm mb-10 leading-6 text-center">
        Enter your Console server URL to connect.
      </Text>

      {/* Environment name */}
      <Text className="text-xs font-medium text-foreground-secondary mb-1.5">Name</Text>
      <TextInput
        value={envName}
        onChangeText={setEnvName}
        placeholder="My server"
        placeholderTextColor="#71717a"
        autoCapitalize="none"
        autoCorrect={false}
        className="bg-card border border-border rounded-xl px-4 py-3.5 text-foreground text-sm mb-4"
      />

      {/* Backend URL */}
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
        onPress={() => saveConnection(envName)}
        disabled={isSaving || !inputUrl.trim()}
        className={`bg-foreground rounded-full py-3.5 items-center justify-center mb-3 ${
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
        className={`border border-border rounded-full py-3.5 items-center justify-center ${
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
    </View>
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
          <View style={{ flex: 1, backgroundColor: theme.colors.background }}>
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
