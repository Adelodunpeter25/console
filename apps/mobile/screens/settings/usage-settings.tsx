import React from "react";
import {
  Text,
  View,
  ScrollView,
  ActivityIndicator,
  RefreshControl,
  Pressable,
} from "react-native";
import { RefreshCw } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { useUsageViewModel } from "@/hooks/useUsageViewModel";
import { UsageProviderCard } from "./components/usage-provider-card";

interface Props {
  onBack?: () => void;
}

export function UsageSettings({ onBack }: Props) {
  const { cards, isLoading, isRefetching, onRefresh } = useUsageViewModel();

  if (isLoading) {
    return (
      <View style={{ flex: 1 }}>
        <ScreenHeader title="Usage" onBack={onBack} />
        <View className="flex-1 items-center justify-center py-16">
          <ActivityIndicator size="small" color="#ffffff" />
          <Text className="text-xs text-foreground-secondary mt-3">Loading quota…</Text>
        </View>
      </View>
    );
  }

  return (
    <View style={{ flex: 1 }}>
      <ScreenHeader
        title="Usage"
        onBack={onBack}
        rightAction={
          <Pressable
            onPress={onRefresh}
            className="w-9 h-9 rounded-full bg-card border border-border items-center justify-center"
            style={({ pressed }) => ({ opacity: pressed ? 0.6 : 1 })}
          >
            <RefreshCw size={16} color="#ffffff" />
          </Pressable>
        }
      />
      <ScrollView
        style={{ flex: 1 }}
        contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 32 }}
        showsVerticalScrollIndicator={false}
        refreshControl={<RefreshControl refreshing={isRefetching} onRefresh={onRefresh} tintColor="#ffffff" />}
      >
        <View className="mb-4 px-1">
          <Text className="text-sm text-foreground-secondary">Remaining quota for your signed-in providers. Pull to refresh.</Text>
        </View>

        {cards.map((card) => (
          <UsageProviderCard
            key={card.key}
            displayName={card.displayName}
            report={card.report}
            loggedIn={card.loggedIn}
            email={card.email}
          />
        ))}

        <View className="mt-2 px-1">
          <Text className="text-[11px] text-foreground-secondary leading-4">
            Antigravity shows Google / Anthropic / OpenAI counters separately. Codex 30d/5h windows share the same account — Spark has a separate meter.
          </Text>
        </View>
      </ScrollView>
    </View>
  );
}

export default UsageSettings;
