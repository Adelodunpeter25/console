import React from "react";
import { View, StyleSheet, ViewStyle } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import { theme } from "../../styles/theme";

interface AppShellProps {
  children: React.ReactNode;
  style?: ViewStyle;
}

export function AppShell({ children, style }: AppShellProps) {
  return (
    <SafeAreaView style={[styles.safeArea, style]}>
      <StatusBar style="light" />
      <View style={styles.content}>
        {children}
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safeArea: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  content: {
    flex: 1,
  },
});
