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
    <ScrollView className="flex-1 bg-[#0d0d0e] px-4 pt-3">
      {/* Header */}
      <View className="mb-4">
        <Text className="text-xl font-bold text-[#f1f3f7] tracking-tight">
          Console Settings
        </Text>
        <Text className="text-xs text-[#9095a0] mt-0.5">
          Configure server connections & app environment
        </Text>
      </View>

      {/* Connection Endpoint Card */}
      <GlassSurface className="mb-4 p-4">
        <View className="flex-row items-center justify-between mb-3">
          <Text className="text-sm font-semibold text-[#f1f3f7]">
            Backend Server Endpoint
          </Text>
          <View className="flex-row items-center gap-1.5 px-2.5 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20">
            <View className="w-1.5 h-1.5 rounded-full bg-emerald-400" />
            <Text className="text-[10px] font-bold text-emerald-400">
              {backendUrl ? "Connected" : "Disconnected"}
            </Text>
          </View>
        </View>

        <Text className="text-xs text-[#9095a0] mb-3">
          HTTP URL of your running Console backend server instance:
        </Text>

        <TextInput
          className="h-11 bg-[#16171a] border border-white/10 rounded-xl px-3 text-[#f1f3f7] text-xs font-mono mb-3"
          value={inputUrl}
          onChangeText={setInputUrl}
          placeholder="http://192.168.1.X:3000"
          placeholderTextColor="#9095a0"
          autoCapitalize="none"
          autoCorrect={false}
        />

        <View className="flex-row gap-2 justify-end">
          <TouchableOpacity
            className="px-4 py-2.5 rounded-full bg-white/10 border border-white/10 items-center justify-center"
            onPress={handleTestConnection}
          >
            <Text className="text-xs font-semibold text-[#f1f3f7]">
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
            className="px-5 py-2.5 rounded-full bg-sky-500 items-center justify-center flex-row gap-2"
            onPress={handleSaveConnection}
            disabled={isSaving}
          >
            {isSaving ? (
              <ActivityIndicator size="small" color="#ffffff" />
            ) : (
              <Text className="text-xs font-bold text-white">Save Changes</Text>
            )}
          </TouchableOpacity>
        </View>
      </GlassSurface>

      {/* App Environment Info Card */}
      <GlassSurface className="mb-4 p-4">
        <Text className="text-sm font-semibold text-[#f1f3f7] mb-3">
          App Info & Diagnostics
        </Text>

        <View className="flex-row justify-between py-2 border-b border-white/5">
          <Text className="text-xs text-[#9095a0]">Console Mobile Version</Text>
          <Text className="text-xs font-mono text-[#f1f3f7]">1.0.0 (Build 2026)</Text>
        </View>

        <View className="flex-row justify-between py-2 border-b border-white/5">
          <Text className="text-xs text-[#9095a0]">Expo Framework</Text>
          <Text className="text-xs font-mono text-[#f1f3f7]">SDK 54</Text>
        </View>

        <View className="flex-row justify-between py-2 border-b border-white/5">
          <Text className="text-xs text-[#9095a0]">React Native Engine</Text>
          <Text className="text-xs font-mono text-[#f1f3f7]">0.81.5 (Hermes)</Text>
        </View>

        <View className="flex-row justify-between py-2">
          <Text className="text-xs text-[#9095a0]">Styling Engine</Text>
          <Text className="text-xs font-mono text-[#38bdf8]">NativeWind v4</Text>
        </View>
      </GlassSurface>

      {/* Danger Zone Card */}
      <GlassSurface className="mb-8 p-4 border-red-500/20 bg-red-500/5">
        <Text className="text-sm font-semibold text-red-400 mb-1">
          Server Connection Reset
        </Text>
        <Text className="text-xs text-[#9095a0] mb-4">
          Clear the saved backend URL and reset connection preferences.
        </Text>

        <TouchableOpacity
          className="py-2.5 px-4 rounded-full bg-red-500/20 border border-red-500/30 items-center justify-center self-start"
          onPress={handleDisconnect}
        >
          <Text className="text-xs font-bold text-red-400">
            Disconnect Backend Endpoint
          </Text>
        </TouchableOpacity>
      </GlassSurface>
    </ScrollView>
  );
}

export default SettingsScreen;
