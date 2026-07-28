import React from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ScrollView,
  ActivityIndicator,
} from "react-native";
import { GlassSurface } from "../../components/common/glass-surface";
import { ScreenHeader } from "../../components/common/screen-header";
import { useServerConnection } from "../../hooks";
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

  return (
    <ScrollView className="flex-1 bg-[#0d0d0e] px-4 pt-4">
      <View className="mb-5">
        <ScreenHeader title="Console Settings" onBack={() => setActiveTab("home")} />
        <Text className="text-sm text-zinc-400 mt-1 ml-4">
          Configure server connections & app environment
        </Text>
      </View>

      {/* Connection Endpoint Card */}
      <GlassSurface className="mb-4 p-5">
        <View className="flex-row items-center justify-between mb-3">
          <Text className="text-base font-semibold text-white">Backend Server Endpoint</Text>
          <View className="flex-row items-center gap-1.5 px-3 py-1 rounded-full bg-white/10 border border-white/20">
            <View className="w-2 h-2 rounded-full bg-white" />
            <Text className="text-xs font-bold text-white">
              {backendUrl ? "Connected" : "Disconnected"}
            </Text>
          </View>
        </View>

        <Text className="text-sm text-zinc-400 mb-4">
          HTTP URL of your running Console backend server instance:
        </Text>

        <TextInput
          className="h-12 bg-[#16171a] border border-white/20 rounded-xl px-4 text-white text-sm font-mono mb-4"
          value={inputUrl}
          onChangeText={setInputUrl}
          placeholder="http://192.168.1.X:3000"
          placeholderTextColor="#71717a"
          autoCapitalize="none"
          autoCorrect={false}
        />

        <View className="flex-row gap-3 justify-end">
          <TouchableOpacity
            className="px-4 py-2.5 rounded-full bg-transparent border border-white/20 items-center justify-center"
            onPress={testConnection}
          >
            <Text className="text-sm font-semibold text-white">
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
            className="px-5 py-2.5 rounded-full bg-white items-center justify-center flex-row gap-2"
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

      {/* App Environment Info Card */}
      <GlassSurface className="mb-4 p-5">
        <Text className="text-base font-semibold text-white mb-3">App Info & Diagnostics</Text>

        <View className="flex-row justify-between py-2.5 border-b border-white/10">
          <Text className="text-sm text-zinc-400">Console Mobile Version</Text>
          <Text className="text-sm font-mono text-white">1.0.0 (Build 2026)</Text>
        </View>

        <View className="flex-row justify-between py-2.5 border-b border-white/10">
          <Text className="text-sm text-zinc-400">Expo Framework</Text>
          <Text className="text-sm font-mono text-white">SDK 54</Text>
        </View>

        <View className="flex-row justify-between py-2.5 border-b border-white/10">
          <Text className="text-sm text-zinc-400">React Native Engine</Text>
          <Text className="text-sm font-mono text-white">0.81.5 (Hermes)</Text>
        </View>

        <View className="flex-row justify-between py-2.5">
          <Text className="text-sm text-zinc-400">State Management</Text>
          <Text className="text-sm font-mono text-white">Zustand v5</Text>
        </View>
      </GlassSurface>

      {/* Danger Zone Card */}
      <GlassSurface className="mb-8 p-5 border-red-500/30 bg-red-500/5">
        <Text className="text-base font-semibold text-red-400 mb-1">Server Connection Reset</Text>
        <Text className="text-sm text-zinc-400 mb-4">
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
