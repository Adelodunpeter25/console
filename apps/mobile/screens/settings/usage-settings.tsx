import React from "react";
import {
  Text,
  View,
  ScrollView,
  ActivityIndicator,
  RefreshControl,
  Pressable,
} from "react-native";
import {
  BarChart3,
  AlertCircle,
  RefreshCw,
  Check,
  Circle,
} from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { GlassSurface } from "@/components/layout/glass-surface";
import { useAllUsage } from "@console/api";
import { useAuth } from "@/hooks";
import { theme } from "@/styles/theme";
import type { UsageLimit, UsageReport } from "@console/types";

function formatResetsAt(resetsAt?: number): string | null {
  if (!resetsAt) return null;
  const diff = resetsAt - Date.now();
  if (diff <= 0) return "resetting…";
  const hours = Math.floor(diff / (1000 * 60 * 60));
  const mins = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return `resets in ${days}d ${hours % 24}h`;
  }
  if (hours > 0) return `resets in ${hours}h ${mins}m`;
  return `resets in ${mins}m`;
}

function formatWindowLabel(limit: UsageLimit): string {
  const windowLabel = limit.window?.label ?? limit.window?.id ?? "Quota";
  const resets = formatResetsAt(limit.window?.resetsAt);
  return resets ? `${windowLabel} · ${resets}` : windowLabel;
}

function statusColor(status?: string): string {
  switch (status) {
    case "exhausted":
      return "#f87171";
    case "warning":
      return "#fbbf24";
    case "ok":
      return "#34d399";
    default:
      return theme.colors.text.muted;
  }
}

function UsageLimitRow({ limit }: { limit: UsageLimit }) {
  const usedPct = limit.amount.usedFraction !== undefined
    ? Math.round(limit.amount.usedFraction * 100)
    : limit.amount.used !== undefined
      ? Math.round(limit.amount.used)
      : null;
  const remainingPct = limit.amount.remainingFraction !== undefined
    ? Math.round(limit.amount.remainingFraction * 1000) / 10
    : null;

  const barPct = limit.amount.usedFraction !== undefined
    ? Math.min(100, Math.max(0, limit.amount.usedFraction * 100))
    : limit.amount.remainingFraction !== undefined
      ? Math.min(100, Math.max(0, (1 - limit.amount.remainingFraction) * 100))
      : 0;

  return (
    <View className="py-3">
      <View className="flex-row items-center justify-between mb-1.5">
        <View className="flex-1 pr-2">
          <Text className="text-xs font-semibold text-foreground" numberOfLines={1}>
            {limit.label}
          </Text>
          <Text className="text-[11px] text-foreground-secondary mt-0.5" numberOfLines={1}>
            {limit.scope.tier ? `${limit.scope.tier} · ` : ""}
            {formatWindowLabel(limit)}
            {limit.scope.modelId ? ` · ${limit.scope.modelId}` : ""}
          </Text>
        </View>
        <View className="items-end">
          <Text className="text-xs font-bold" style={{ color: statusColor(limit.status) }}>
            {usedPct !== null ? `${usedPct}% used` : remainingPct !== null ? `${remainingPct}% left` : limit.status ?? "—"}
          </Text>
          {limit.amount.remaining !== undefined && limit.amount.remainingFraction !== undefined && (
            <Text className="text-[11px] text-foreground-secondary">
              {limit.amount.remaining.toFixed(1)}% remaining
            </Text>
          )}
        </View>
      </View>
      <View className="h-1.5 rounded-full overflow-hidden" style={{ backgroundColor: "rgba(255,255,255,0.1)" }}>
        <View
          className="h-full rounded-full"
          style={{
            width: `${barPct}%` as unknown as number,
            backgroundColor: statusColor(limit.status),
          }}
        />
      </View>
    </View>
  );
}

function UsageProviderCard({
  provider,
  displayName,
  report,
  loggedIn,
  email,
}: {
  provider: string;
  displayName: string;
  report: UsageReport | null | undefined;
  loggedIn: boolean;
  email?: string;
}) {
  const isExhausted = report?.limits.some((l) => l.status === "exhausted");
  const mostPressured = report?.limits[0];

  return (
    <GlassSurface className="p-4 mb-3">
      <View className="flex-row items-center justify-between mb-2">
        <View className="flex-row items-center gap-2 flex-1">
          {loggedIn ? (
            <Check size={14} color={isExhausted ? "#f87171" : "#34d399"} />
          ) : (
            <Circle size={14} color={theme.colors.text.muted} />
          )}
          <View className="flex-1">
            <Text className="text-sm font-semibold text-foreground">{displayName}</Text>
            <Text className="text-[11px] text-foreground-secondary mt-0.5" numberOfLines={1}>
              {loggedIn ? (email ?? "Connected") : "Not connected"}
              {report?.metadata?.currentTierId ? ` · ${String(report.metadata.currentTierId)}` : ""}
              {isExhausted ? " · quota exhausted" : ""}
            </Text>
          </View>
        </View>
        {mostPressured && (
          <Text className="text-xs font-bold" style={{ color: statusColor(mostPressured.status) }}>
            {mostPressured.amount.used !== undefined ? `${Math.round(mostPressured.amount.used)}%` : ""}
          </Text>
        )}
      </View>

      {!loggedIn ? (
        <View className="py-3 items-center">
          <Text className="text-xs text-foreground-secondary text-center">
            Sign in via Account to see quota.
          </Text>
        </View>
      ) : !report ? (
        <View className="flex-row items-center gap-2 py-3">
          <AlertCircle size={14} color="#fbbf24" />
          <Text className="text-xs text-foreground-secondary flex-1">
            Quota unavailable — token expired, project missing, or billing disabled. Re-login in Account.
          </Text>
        </View>
      ) : report.limits.length === 0 ? (
        <View className="py-3">
          <Text className="text-xs text-foreground-secondary">No limits reported.</Text>
        </View>
      ) : (
        <View className="mt-1">
          {report.limits.map((limit) => (
            <UsageLimitRow key={limit.id} limit={limit} />
          ))}
          {report.metadata?.projectId ? (
            <Text className="text-[10px] text-foreground-secondary mt-2 font-mono">
              project: {String(report.metadata.projectId)}
            </Text>
          ) : null}
        </View>
      )}
    </GlassSurface>
  );
}

interface UsageSettingsProps {
  onBack?: () => void;
}

export function UsageSettings({ onBack }: UsageSettingsProps) {
  const auth = useAuth();
  const { data: allUsage, isLoading, refetch, isRefetching } = useAllUsage();

  const geminiReport = allUsage?.gemini ?? null;
  const antigravityReport = allUsage?.antigravity ?? null;
  const codexReport = allUsage?.codex ?? null;

  const onRefresh = React.useCallback(() => {
    void refetch();
  }, [refetch]);

  if (isLoading && !allUsage) {
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
        refreshControl={
          <RefreshControl refreshing={isRefetching} onRefresh={onRefresh} tintColor="#ffffff" />
        }
      >
        <View className="mb-4 px-1">
          <Text className="text-sm text-foreground-secondary">
            Remaining quota for your signed-in providers. Pull to refresh.
          </Text>
        </View>

        <UsageProviderCard
          provider="antigravity"
          displayName="Google Antigravity"
          report={antigravityReport}
          loggedIn={Boolean(auth.status?.antigravity?.loggedIn)}
          email={auth.status?.antigravity?.email ?? undefined}
        />

        <UsageProviderCard
          provider="gemini"
          displayName="Google Gemini"
          report={geminiReport}
          loggedIn={Boolean(auth.status?.gemini?.loggedIn)}
          email={auth.status?.gemini?.email ?? undefined}
        />

        <UsageProviderCard
          provider="codex"
          displayName="OpenAI Codex"
          report={codexReport}
          loggedIn={Boolean(auth.status?.codex?.loggedIn)}
          email={auth.status?.codex?.email ?? undefined}
        />

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
