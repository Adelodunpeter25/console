import React from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  ScrollView,
  ActivityIndicator,
} from "react-native";
import { GlassSurface } from "../../components/layout/glass-surface";
import { useServerConnection } from "../../hooks";

export function ConnectionSettings() {
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

  return (
    <ScrollView
      style={{ flex: 1 }}
      contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 40 }}
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="on-drag"
      showsVerticalScrollIndicator={false}
    >
      {/* Status badge */}
      <View className="flex-row items-center justify-between mb-4 px-1">
        <Text className="text-sm text-foreground-secondary">Connection status</Text>
        <View className="flex-row items-center gap-1.5 px-3 py-1 rounded-full bg-foreground/10 border border-border">
          <View
            className={`w-2 h-2 rounded-full ${backendUrl ? "bg-emerald-400" : "bg-zinc-500"}`}
          />
          <Text className="text-xs font-bold text-foreground">
            {backendUrl ? "Connected" : "Disconnected"}
          </Text>
        </View>
      </View>

      {/* Endpoint editor */}
      <GlassSurface className="mb-4 p-5">
        <Text className="text-base font-semibold text-foreground mb-2">Backend Endpoint</Text>
        <Text className="text-sm text-foreground-secondary mb-4">
          HTTP URL of your running Console backend server instance.
        </Text>

        <TextInput
          className="h-12 bg-background border border-border rounded-xl px-4 text-foreground text-sm font-mono mb-4"
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

      {/* Disconnect — moved here from the old danger zone */}
      <GlassSurface className="p-5 border-red-500/30 bg-red-500/5">
        <Text className="text-base font-semibold text-red-400 mb-1">Disconnect</Text>
        <Text className="text-sm text-foreground-secondary mb-4">
          Clear the saved backend URL and reset connection preferences.
        </Text>

        <TouchableOpacity
          className="py-2.5 px-5 rounded-full bg-transparent border border-red-500/40 items-center justify-center self-start"
          onPress={disconnect}
        >
          <Text className="text-sm font-bold text-red-400">Disconnect Backend</Text>
        </TouchableOpacity>
      </GlassSurface>
    </ScrollView>
  );
}

export default ConnectionSettings;
