import React from "react";
import { View, Text } from "react-native";
import type { ToolResult } from "@console/types";
import { theme } from "../../styles/theme";

/** Compact live tool-result rows shown under the streaming bubble. */
export function LiveToolResults({ results }: { results: ToolResult[] }) {
  if (results.length === 0) return null;
  return (
    <View className="w-full max-w-[92%] mb-3.5 self-start gap-1.5">
      {results.map((result, idx) => (
        <View key={idx} className="bg-card-alt border border-border rounded-lg p-2.5">
          <View className="flex-row justify-between items-center mb-1">
            <Text className="text-xs font-mono font-bold text-foreground">⚙️ {result.toolName}</Text>
            <Text
              className="text-[9px] font-bold font-mono tracking-wide"
              style={{ color: result.isError ? theme.colors.status.attention : theme.colors.status.ready }}
            >
              {result.isError ? "FAILED" : "DONE"}
            </Text>
          </View>
          <Text className="text-xs font-mono text-foreground-secondary leading-4" numberOfLines={3} selectable>
            {String(result.content || "")}
          </Text>
        </View>
      ))}
    </View>
  );
}
