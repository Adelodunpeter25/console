import React from "react";
import { View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import { KeyboardProvider } from "react-native-keyboard-controller";

interface AppShellProps {
  children: React.ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  return (
    <KeyboardProvider>
      <SafeAreaView style={{ flex: 1, backgroundColor: "#0a0a0b" }}>
        <StatusBar style="light" />
        <View style={{ flex: 1 }}>{children}</View>
      </SafeAreaView>
    </KeyboardProvider>
  );
}
