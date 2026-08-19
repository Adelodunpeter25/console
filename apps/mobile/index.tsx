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
import { KeyboardProvider } from "react-native-keyboard-controller";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { BottomSheetModalProvider } from "@gorhom/bottom-sheet";
import { MainContent } from "./components/layout/main-content";
import { ConfirmDialog } from "./components/common/confirm-dialog";
import { useServerConnection } from "./hooks";

// QueryClient tuned for mobile: 5-minute staleTime (no refetch-on-mount
// within that window) and refetchOnWindowFocus disabled (prevents a full
// refetch storm every time the app foregrounds). Imported directly from
// @tanstack/react-query — NOT via the @console/api factory — so Metro
// resolves react-query against the same React copy the app uses. Importing
// the factory from @console/api shifted the module graph and left
// react-query's `React` default import null inside ConsoleApiProvider
// ("Cannot read property 'useEffect' of null").
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 5,
      retry: 2,
      refetchOnWindowFocus: false,
    },
  },
});

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
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <View style={{ flex: 1, backgroundColor: "#0a0a0b" }}>
          <StatusBar style="light" />

          {backendUrl ? (
            <ConsoleApiProvider baseUrl={backendUrl} queryClient={queryClient}>
              <KeyboardProvider>
                <BottomSheetModalProvider>
                  <MainContent />
                </BottomSheetModalProvider>
              </KeyboardProvider>
            </ConsoleApiProvider>
          ) : (
            <OnboardingScreen />
          )}

          <ConfirmDialog />
        </View>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}

registerRootComponent(AppRoot);
export default AppRoot;
