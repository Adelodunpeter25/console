import React from "react";
import { View } from "react-native";
import type { ToolResult } from "@console/types";
import { ToolResultItem } from "./message-bubbles";

/** Live tool-result rows shown under the streaming bubble. */
export function LiveToolResults({ results }: { results: ToolResult[] }) {
  if (results.length === 0) return null;
  return (
    <View className="mb-4">
      {results.map((result, idx) => (
        <ToolResultItem key={idx} result={result} />
      ))}
    </View>
  );
}