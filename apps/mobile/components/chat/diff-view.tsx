import React, { memo, useState, useMemo } from "react";
import { View, Text, Pressable, ScrollView } from "react-native";
import { ChevronDown, ChevronUp } from "lucide-react-native";
import type { DiffResult, DiffLine } from "../../utils/diff";
import { getFileName } from "../../utils/tool-helpers";
import { getLanguageFromPath, renderHighlightedLine } from "../common/syntax-highlighter";

interface DiffViewProps {
  diff: DiffResult;
  /** Maximum number of lines to render before showing a toggle to expand full diff. Default 60. */
  maxCollapsedLines?: number;
  filePath?: string;
}

const DiffLineRow = memo(function DiffLineRow({
  line,
  language,
  lineIndex,
}: {
  line: DiffLine;
  language?: string;
  lineIndex: number;
}) {
  const isAdded = line.type === "added";
  const isRemoved = line.type === "removed";

  const rowBg = isAdded
    ? "bg-emerald-500/10"
    : isRemoved
      ? "bg-red-500/10"
      : "bg-transparent";

  const gutterColor = isAdded
    ? "text-emerald-400 font-bold"
    : isRemoved
      ? "text-red-400 font-bold"
      : "text-foreground-secondary/40";

  const gutterChar = isAdded ? "+" : isRemoved ? "-" : " ";

  const oldNo = line.oldLineNo != null ? String(line.oldLineNo) : "";
  const newNo = line.newLineNo != null ? String(line.newLineNo) : "";

  const highlighted = useMemo(
    () => renderHighlightedLine(line.text, language, `dl-${lineIndex}`),
    [line.text, language, lineIndex],
  );

  return (
    <View className={`flex-row items-stretch py-0.5 px-2 ${rowBg}`}>
      {/* Line Numbers Gutter */}
      <View className="flex-row items-center w-14 shrink-0 select-none mr-2">
        <Text className="w-6 text-[10px] font-mono text-foreground-secondary/40 text-right pr-1">
          {oldNo}
        </Text>
        <Text className="w-6 text-[10px] font-mono text-foreground-secondary/40 text-right pr-1">
          {newNo}
        </Text>
        <Text className={`w-2 text-[10px] font-mono ${gutterColor}`}>
          {gutterChar}
        </Text>
      </View>

      {/* Code Text with Syntax Highlighting */}
      <Text
        className="text-[11px] font-mono leading-4 flex-1"
        style={{
          opacity: isRemoved ? 0.75 : 1,
          color: isAdded ? "#4ade80" : isRemoved ? "#f87171" : "#e4e4e7",
        }}
        selectable
      >
        {highlighted}
      </Text>
    </View>
  );
});

export const DiffSummaryBadge = memo(function DiffSummaryBadge({
  addedCount,
  removedCount,
}: {
  addedCount: number;
  removedCount: number;
}) {
  return (
    <View className="flex-row items-center gap-1.5 shrink-0">
      {addedCount > 0 && (
        <Text className="text-[11px] font-mono font-semibold text-emerald-400">
          +{addedCount}
        </Text>
      )}
      {removedCount > 0 && (
        <Text className="text-[11px] font-mono font-semibold text-red-400">
          -{removedCount}
        </Text>
      )}
      {addedCount === 0 && removedCount === 0 && (
        <Text className="text-[11px] font-mono text-foreground-secondary/50">
          +0 -0
        </Text>
      )}
    </View>
  );
});

export const DiffView = memo(function DiffView({
  diff,
  maxCollapsedLines = 60,
  filePath,
}: DiffViewProps) {
  const [expanded, setExpanded] = useState(false);
  const language = useMemo(() => getLanguageFromPath(filePath), [filePath]);
  const totalLines = diff.lines.length;
  const isTruncated = totalLines > maxCollapsedLines;
  const visibleLines = isTruncated && !expanded
    ? diff.lines.slice(0, maxCollapsedLines)
    : diff.lines;

  if (totalLines === 0) {
    return (
      <View className="p-3 bg-black/40 rounded-xl">
        <Text className="text-xs font-mono text-foreground-secondary/60 italic">
          No changes detected.
        </Text>
      </View>
    );
  }

  return (
    <View className="rounded-xl overflow-hidden bg-black/50 border border-white/[0.08]">
      {/* Header Bar */}
      <View className="flex-row items-center justify-between px-3 py-2 bg-white/[0.03] border-b border-white/[0.06]">
        {filePath ? (
          <Text className="text-xs font-mono text-foreground font-medium flex-1 mr-2" numberOfLines={1}>
            {getFileName(filePath)}
          </Text>
        ) : (
          <Text className="text-xs font-mono text-foreground-secondary/70">
            Changes
          </Text>
        )}
        <DiffSummaryBadge
          addedCount={diff.addedCount}
          removedCount={diff.removedCount}
        />
      </View>

      {/* Diff Lines Scrollable Container */}
      <ScrollView horizontal showsHorizontalScrollIndicator={false}>
        <View className="min-w-full py-1">
          {visibleLines.map((line, idx) => (
            <DiffLineRow
              key={`${line.type}-${idx}-${line.oldLineNo}-${line.newLineNo}`}
              line={line}
              language={language}
              lineIndex={idx}
            />
          ))}
        </View>
      </ScrollView>

      {/* Expansion Toggle */}
      {isTruncated && (
        <Pressable
          onPress={() => setExpanded(!expanded)}
          className="flex-row items-center justify-center gap-1 py-2 bg-white/[0.04] border-t border-white/[0.06]"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        >
          <Text className="text-xs font-medium text-foreground-secondary">
            {expanded
              ? "Show less"
              : `Show full diff (+${totalLines - maxCollapsedLines} lines)`}
          </Text>
          {expanded ? (
            <ChevronUp size={13} color="#a1a1aa" />
          ) : (
            <ChevronDown size={13} color="#a1a1aa" />
          )}
        </Pressable>
      )}
    </View>
  );
});
