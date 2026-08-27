import React from "react";
import { Text, View } from "react-native";
import { AlertCircle, Check, Circle } from "lucide-react-native";
import { GlassSurface } from "@/components/layout/glass-surface";
import { theme } from "@/styles/theme";
import { statusColor } from "@/utils/usage-helpers";
import { UsageLimitRow } from "./usage-limit-row";
import type { UsageReport } from "@console/types";

interface Props {
  displayName: string;
  report: UsageReport | null | undefined;
  loggedIn: boolean;
  email?: string;
}

export function UsageProviderCard({ displayName, report, loggedIn, email }: Props) {
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
          <Text className="text-xs text-foreground-secondary text-center">Sign in via Account to see quota.</Text>
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
