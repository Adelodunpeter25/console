import React from "react";
import { Text, View } from "react-native";
import type { UsageLimit } from "@console/types";
import {
  colorForLimit,
  formatWindowLabel,
  getBarPercent,
  getUsedPercent,
} from "@/utils/usage-helpers";

interface Props {
  limit: UsageLimit;
}

export function UsageLimitRow({ limit }: Props) {
  const usedPct = getUsedPercent(limit);
  const remainingPct =
    limit.amount.remainingFraction !== undefined
      ? Math.round(limit.amount.remainingFraction * 1000) / 10
      : null;
  const barPct = getBarPercent(limit);

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
          <Text className="text-xs font-bold" style={{ color: colorForLimit(limit) }}>
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
            backgroundColor: colorForLimit(limit),
          }}
        />
      </View>
    </View>
  );
}
