import React, { useState } from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ScrollView,
  ActivityIndicator,
} from "react-native";
import * as Linking from "expo-linking";
import { useProviderModels } from "@console/api";
import { GlassSurface } from "../../components/layout/glass-surface";
import { ScreenHeader } from "../../components/layout/screen-header";
import { useServerConnection } from "../../hooks";
import { useAuth } from "../../hooks";
import { useProviderCatalog } from "../../hooks";
import { useAppStore } from "../../stores";

export function SettingsScreen() {
  const {
    backendUrl,
    inputUrl,
    setInputUrl,
    isSaving,
    testingStatus,
    saveConnection,
    testConnection,
    disconnect,
  } = useServerConnection();
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const auth = useAuth();
  const catalog = useProviderCatalog();
  const [selectedProvider, setSelectedProvider] = useState<"gemini" | "antigravity">("antigravity");

  const handleLogin = async () => {
    try {
      const url = await auth.getLoginUrlFor(selectedProvider);
      Linking.openURL(url);
    } catch (err) {
      console.error("Failed to open login URL:", err);
    }
  };

  const modelsData = useProviderModels(selectedProvider);
  const models = modelsData.data?.models ?? [];
  const loadingModels = modelsData.isLoading;

  return (
    <ScrollView className="flex-1 bg-screen px-4 pt-4" style={{ flex: 1 }}>
      <View className="mb-5">
        <ScreenHeader title="Console Settings" onBack={() => setActiveTab("home")} />
        <Text className="text-sm text-foreground-secondary mt-1 ml-4">
          Configure server connections & app environment
        </Text>
      </View>

      {/* Connection Endpoint Card */}
      <GlassSurface className="mb-4 p-5">
        <View className="flex-row items-center justify-between mb-3">
          <Text className="text-base font-semibold text-foreground">Backend Server Endpoint</Text>
          <View className="flex-row items-center gap-1.5 px-3 py-1 rounded-full bg-foreground/10 border border-border">
            <View className="w-2 h-2 rounded-full bg-foreground" />
            <Text className="text-xs font-bold text-foreground">
              {backendUrl ? "Connected" : "Disconnected"}
            </Text>
          </View>
        </View>

        <Text className="text-sm text-foreground-secondary mb-4">
          HTTP URL of your running Console backend server instance:
        </Text>

        <TextInput
          className="h-12 bg-card border border-border rounded-xl px-4 text-foreground text-sm font-mono mb-4"
          value={inputUrl}
          onChangeText={setInputUrl}
          placeholder="http://192.168.1.X:3000"
          placeholderTextColor="#71717a"
          autoCapitalize="none"
          autoCorrect={false}
        />

        <View className="flex-row gap-3 justify-end">
          <TouchableOpacity
            className="px-4 py-2.5 rounded-full bg-transparent border border-border items-center justify-center"
            onPress={testConnection}
          >
            <Text className="text-sm font-semibold text-foreground">
              {testingStatus === "testing"
                ? "Testing..."
                : testingStatus === "success"
                  ? "✅ Online"
                  : testingStatus === "error"
                    ? "❌ Offline"
                    : "Test Connection"}
            </Text>
          </TouchableOpacity>

          <TouchableOpacity
            className="px-5 py-2.5 rounded-full bg-foreground items-center justify-center flex-row gap-2"
            onPress={saveConnection}
            disabled={isSaving}
          >
            {isSaving ? (
              <ActivityIndicator size="small" color="#000000" />
            ) : (
              <Text className="text-sm font-bold text-black">Save Changes</Text>
            )}
          </TouchableOpacity>
        </View>
      </GlassSurface>

      {/* Provider / Model Card */}
      <GlassSurface className="mb-4 p-5">
        <Text className="text-base font-semibold text-foreground mb-3">Provider & Model</Text>

        <Text className="text-xs font-bold text-foreground-secondary uppercase tracking-wider mb-2">
          Provider
        </Text>
        <View className="flex-row gap-2 mb-4">
          {catalog.providers.map((p) => {
            const active = selectedProvider === p.name;
            return (
              <TouchableOpacity
                key={p.name}
                className={`flex-1 py-2.5 rounded-full items-center justify-center border ${
                  active ? "bg-foreground border-foreground" : "bg-transparent border-border"
                }`}
                onPress={() => setSelectedProvider(p.name)}
              >
                <Text className={`text-sm font-bold ${active ? "text-black" : "text-foreground"}`}>
                  {p.name}
                </Text>
              </TouchableOpacity>
            );
          })}
        </View>

        <Text className="text-xs font-bold text-foreground-secondary uppercase tracking-wider mb-2">
          Model
        </Text>
        {catalog.loadingProviders || loadingModels ? (
          <ActivityIndicator size="small" color="#ffffff" />
        ) : (
          <View className="gap-1.5">
            {models.map((m) => (
              <View key={m.id} className="py-2 px-3 rounded-lg bg-card border border-border">
                <Text className="text-sm text-foreground">{m.id}</Text>
              </View>
            ))}
          </View>
        )}
      </GlassSurface>

      {/* Account / OAuth Card */}
      <GlassSurface className="mb-4 p-5">
        <Text className="text-base font-semibold text-foreground mb-3">Account</Text>
        {catalog.providers.map((p) => {
          const provider = p.name;
          const loggedIn = auth.isLoggedIn(provider);
          return (
            <View
              key={provider}
              className="flex-row items-center justify-between py-2.5 border-b border-border last:border-b-0"
            >
              <View className="flex-1 pr-3">
                <Text className="text-sm font-semibold text-foreground">{p.name}</Text>
                <Text className="text-xs text-foreground-secondary mt-0.5">
                  {loggedIn ? (auth.status?.[provider]?.email ?? "Logged in") : "Not logged in"}
                </Text>
              </View>
              {loggedIn ? (
                <View className="px-3 py-1 rounded-full bg-foreground/10 border border-border">
                  <Text className="text-xs font-bold text-foreground">✓ Connected</Text>
                </View>
              ) : (
                <TouchableOpacity
                  className="px-4 py-2 rounded-full bg-foreground items-center justify-center"
                  onPress={handleLogin}
                  disabled={auth.isFetchingLoginUrl}
                >
                  {auth.isFetchingLoginUrl ? (
                    <ActivityIndicator size="small" color="#000000" />
                  ) : (
                    <Text className="text-xs font-bold text-black">Log In</Text>
                  )}
                </TouchableOpacity>
              )}
            </View>
          );
        })}
      </GlassSurface>

      {/* App Info Card */}
      <GlassSurface className="mb-4 p-5">
        <Text className="text-base font-semibold text-foreground mb-3">App Info & Diagnostics</Text>

        <View className="flex-row justify-between py-2.5 border-b border-border">
          <Text className="text-sm text-foreground-secondary">Console Mobile Version</Text>
          <Text className="text-sm font-mono text-foreground">1.0.0 (Build 2026)</Text>
        </View>

        <View className="flex-row justify-between py-2.5 border-b border-border">
          <Text className="text-sm text-foreground-secondary">Expo Framework</Text>
          <Text className="text-sm font-mono text-foreground">SDK 54</Text>
        </View>

        <View className="flex-row justify-between py-2.5 border-b border-border">
          <Text className="text-sm text-foreground-secondary">React Native Engine</Text>
          <Text className="text-sm font-mono text-foreground">0.81.5 (Hermes)</Text>
        </View>

        <View className="flex-row justify-between py-2.5">
          <Text className="text-sm text-foreground-secondary">State Management</Text>
          <Text className="text-sm font-mono text-foreground">Zustand v5</Text>
        </View>
      </GlassSurface>

      {/* Danger Zone Card */}
      <GlassSurface className="mb-8 p-5 border-red-500/30 bg-red-500/5">
        <Text className="text-base font-semibold text-red-400 mb-1">Server Connection Reset</Text>
        <Text className="text-sm text-foreground-secondary mb-4">
          Clear the saved backend URL and reset connection preferences.
        </Text>

        <TouchableOpacity
          className="py-2.5 px-5 rounded-full bg-transparent border border-red-500/40 items-center justify-center self-start"
          onPress={disconnect}
        >
          <Text className="text-sm font-bold text-red-400">Disconnect Backend Endpoint</Text>
        </TouchableOpacity>
      </GlassSurface>
    </ScrollView>
  );
}

export default SettingsScreen;
