import React from "react";
import { Text, View, ScrollView } from "react-native";
import Constants from "expo-constants";
import { GlassSurface } from "../../components/layout/glass-surface";

export function AboutSettings() {
  const version = Constants.expoConfig?.version ?? "1.0.0";

  return (
    <ScrollView
      style={{ flex: 1 }}
      contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 40 }}
      showsVerticalScrollIndicator={false}
    >
      <View className="mb-4 px-1">
        <Text className="text-sm text-foreground-secondary">
          Console Mobile — your local AI coding companion.
        </Text>
      </View>

      <GlassSurface className="p-5">
        <View className="flex-row justify-between py-3 border-b border-border">
          <Text className="text-sm text-foreground-secondary">App Version</Text>
          <Text className="text-sm font-mono text-foreground">{version}</Text>
        </View>
        <View className="flex-row justify-between py-3">
          <Text className="text-sm text-foreground-secondary">Platform</Text>
          <Text className="text-sm font-mono text-foreground">{Constants.platform?.ios ? "iOS" : "Android"}</Text>
        </View>
      </GlassSurface>
    </ScrollView>
  );
}

export default AboutSettings;
