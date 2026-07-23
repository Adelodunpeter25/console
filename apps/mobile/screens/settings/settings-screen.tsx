import React, { useState } from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ScrollView,
  Alert,
  ActivityIndicator,
} from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { configureConsoleApi } from "@console/api";
import { GlassSurface } from "../../components/common/glass-surface";

const BACKEND_URL_KEY = "@console_backend_url";

interface SettingsScreenProps {
  backendUrl: string | null;
  setBackendUrl: (url: string | null) => void;
  setActiveTab: (tab: "home" | "chat" | "settings") => void;
}

export function SettingsScreen({
  backendUrl,
  setBackendUrl,
  setActiveTab,
}: SettingsScreenProps) {
  const [inputUrl, setInputUrl] = useState(backendUrl || "http://localhost:3000");
  const [isSaving, setIsSaving] = useState(false);
  const [testingStatus, setTestingStatus] = useState<string | null>(null);

  const handleSaveConnection = async () => {
    if (!inputUrl.trim()) {
      Alert.alert("Invalid URL", "Backend server endpoint cannot be empty.");
      return;
    }
    setIsSaving(true);
    try {
      const url = inputUrl.trim();
      await AsyncStorage.setItem(BACKEND_URL_KEY, url);
      configureConsoleApi({ baseUrl: url });
      setBackendUrl(url);
      Alert.alert("Success", "Console backend server endpoint updated successfully!");
    } catch {
      Alert.alert("Error", "Failed to save endpoint URL.");
    } finally {
      setIsSaving(false);
    }
  };

  const handleTestConnection = async () => {
    if (!inputUrl.trim()) return;
    setTestingStatus("testing");
    try {
      const res = await fetch(`${inputUrl.trim()}/api/projects`);
      if (res.ok) {
        setTestingStatus("success");
      } else {
        setTestingStatus("error");
      }
    } catch {
      setTestingStatus("error");
    }
  };

  const handleDisconnect = async () => {
    Alert.alert(
      "Disconnect Backend",
      "Are you sure you want to clear the saved backend URL?",
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Disconnect",
          style: "destructive",
          onPress: async () => {
            await AsyncStorage.removeItem(BACKEND_URL_KEY);
            setBackendUrl(null);
            setActiveTab("home");
          },
        },
      ]
    );
  };

  return (
    <ScrollView className="flex-1 bg-[#0d0d0e] px-4 pt-4">
      {/* Header */}
      <View className="mb-5">
        <Text className="text-2xl font-bold text-white tracking-tight">
          Console Settings
        </Text>
        <Text className="text-sm text-zinc-400 mt-1">
          Configure server connections & app environment
        </Text>
      </View>

      {/* Connection Endpoint Card */}
      <GlassSurface className="mb-4 p-5">
        <View className="flex-row items-center justify-between mb-3">
          <Text className="text-base font-semibold text-white">
            Backend Server Endpoint
          </Text>
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
            onPress={handleTestConnection}
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
            onPress={handleSaveConnection}
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
        <Text className="text-base font-semibold text-white mb-3">
          App Info & Diagnostics
        </Text>

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
          <Text className="text-sm text-zinc-400">Styling Engine</Text>
          <Text className="text-sm font-mono text-white">NativeWind v4</Text>
        </View>
      </GlassSurface>

      {/* Danger Zone Card */}
      <GlassSurface className="mb-8 p-5 border-red-500/30 bg-red-500/5">
        <Text className="text-base font-semibold text-red-400 mb-1">
          Server Connection Reset
        </Text>
        <Text className="text-sm text-zinc-400 mb-4">
          Clear the saved backend URL and reset connection preferences.
        </Text>

        <TouchableOpacity
          className="py-2.5 px-5 rounded-full bg-transparent border border-red-500/40 items-center justify-center self-start"
          onPress={handleDisconnect}
        >
          <Text className="text-sm font-bold text-red-400">
            Disconnect Backend Endpoint
          </Text>
        </TouchableOpacity>
      </GlassSurface>
    </ScrollView>
  );
}

export default SettingsScreen;
