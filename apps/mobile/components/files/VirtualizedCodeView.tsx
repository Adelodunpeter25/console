import React, { memo, useCallback, useMemo, useState } from "react";
import { View, Text, ScrollView, Dimensions, Pressable } from "react-native";
import { LegendList } from "@legendapp/list/react-native";
import * as Clipboard from "expo-clipboard";
import { Check, Copy, X } from "lucide-react-native";
import type { ThemedToken } from "@/services/highlighter";
import { theme } from "@/styles/theme";

interface LineItem {
  lineNumber: number;
  tokens: ThemedToken[];
}

const LINE_HEIGHT = 20;
const CHAR_WIDTH_APPROX = 7.0;

const CodeLine = memo(function CodeLine({
  lineNumber,
  tokens,
  gutterWidth,
  isSelected,
  onToggleSelect,
}: {
  lineNumber: number;
  tokens: ThemedToken[];
  gutterWidth: number;
  isSelected: boolean;
  onToggleSelect: (lineNum: number) => void;
}) {
  return (
    <View
      style={{
        flexDirection: "row",
        height: LINE_HEIGHT,
        alignItems: "center",
        backgroundColor: isSelected ? "rgba(56, 189, 248, 0.16)" : "transparent",
      }}
    >
      {/* Gutter Line Number - tap to select/deselect line */}
      <Pressable
        hitSlop={{ top: 2, bottom: 2, left: 4, right: 4 }}
        onPress={() => onToggleSelect(lineNumber)}
        style={{
          width: gutterWidth,
          paddingRight: 10,
          alignItems: "flex-end",
          justifyContent: "center",
        }}
      >
        <Text
          selectable={false}
          style={{
            fontSize: 10.5,
            lineHeight: LINE_HEIGHT,
            fontFamily: "JetBrainsMono",
            color: isSelected ? "#38bdf8" : "rgba(255, 255, 255, 0.3)",
            fontWeight: isSelected ? "600" : "normal",
          }}
        >
          {lineNumber}
        </Text>
      </Pressable>

      {/* Code Text with native selection enabled per line */}
      <Text
        selectable
        selectionColor="rgba(56, 189, 248, 0.35)"
        style={{
          fontSize: 11,
          lineHeight: LINE_HEIGHT,
          fontFamily: "JetBrainsMono",
          color: theme.colors.text.primary,
        }}
      >
        {tokens.map((token, j) => {
          const isBold = token.fontStyle === 2;
          const isItalic = token.fontStyle === 1;
          return (
            <Text
              key={j}
              style={{
                fontFamily: isBold ? "JetBrainsMono-Bold" : "JetBrainsMono",
                color: token.color || theme.colors.text.primary,
                fontStyle: isItalic ? "italic" : "normal",
              }}
            >
              {token.content}
            </Text>
          );
        })}
      </Text>
    </View>
  );
});

export interface VirtualizedCodeViewProps {
  tokens: ThemedToken[][];
  bottomInset?: number;
}

export const VirtualizedCodeView = memo(function VirtualizedCodeView({
  tokens,
  bottomInset = 0,
}: VirtualizedCodeViewProps) {
  const windowWidth = Dimensions.get("window").width;
  const [selectedLines, setSelectedLines] = useState<Set<number>>(() => new Set());
  const [lastAnchor, setLastAnchor] = useState<number | null>(null);
  const [copiedSelected, setCopiedSelected] = useState(false);

  const totalLines = tokens.length;
  // Compute gutter width dynamically based on number of digits
  const digits = String(totalLines || 1).length;
  const gutterWidth = Math.max(30, digits * 8 + 14);

  // Compute maximum line length so the horizontal scroll view has the exact right content width
  const contentWidth = useMemo(() => {
    let maxChars = 0;
    for (let i = 0; i < tokens.length; i++) {
      const line = tokens[i];
      if (!line) continue;
      let len = 0;
      for (let j = 0; j < line.length; j++) {
        len += line[j]?.content.length ?? 0;
      }
      if (len > maxChars) maxChars = len;
    }
    return Math.max(windowWidth, gutterWidth + maxChars * CHAR_WIDTH_APPROX + 48);
  }, [tokens, windowWidth, gutterWidth]);

  const items = useMemo<LineItem[]>(() => {
    return tokens.map((lineTokens, i) => ({
      lineNumber: i + 1,
      tokens: lineTokens,
    }));
  }, [tokens]);

  const handleToggleSelect = useCallback(
    (lineNum: number) => {
      setSelectedLines((prev) => {
        const next = new Set(prev);
        // Range selection if another line was selected recently and this is a second tap
        if (lastAnchor !== null && lastAnchor !== lineNum && !prev.has(lineNum)) {
          const start = Math.min(lastAnchor, lineNum);
          const end = Math.max(lastAnchor, lineNum);
          for (let i = start; i <= end; i++) {
            next.add(i);
          }
          setLastAnchor(lineNum);
          return next;
        }

        if (next.has(lineNum)) {
          next.delete(lineNum);
          if (next.size === 0) setLastAnchor(null);
        } else {
          next.add(lineNum);
          setLastAnchor(lineNum);
        }
        return next;
      });
    },
    [lastAnchor],
  );

  const handleCopySelected = useCallback(() => {
    if (selectedLines.size === 0) return;
    const sortedLines = Array.from(selectedLines).sort((a, b) => a - b);
    const textToCopy = sortedLines
      .map((lineNum) => {
        const lineTokens = tokens[lineNum - 1] || [];
        return lineTokens.map((t) => t.content).join("");
      })
      .join("\n");

    void Clipboard.setStringAsync(textToCopy);
    setCopiedSelected(true);
    setTimeout(() => setCopiedSelected(false), 2000);
  }, [selectedLines, tokens]);

  const handleClearSelection = useCallback(() => {
    setSelectedLines(new Set());
    setLastAnchor(null);
  }, []);

  const renderItem = useCallback(
    ({ item }: { item: LineItem }) => {
      const isSelected = selectedLines.has(item.lineNumber);
      return (
        <CodeLine
          lineNumber={item.lineNumber}
          tokens={item.tokens}
          gutterWidth={gutterWidth}
          isSelected={isSelected}
          onToggleSelect={handleToggleSelect}
        />
      );
    },
    [gutterWidth, selectedLines, handleToggleSelect],
  );

  const keyExtractor = useCallback((item: LineItem) => `line-${item.lineNumber}`, []);

  return (
    <View style={{ flex: 1 }}>
      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        style={{ flex: 1 }}
        contentContainerStyle={{ width: contentWidth, minWidth: "100%" }}
      >
        <LegendList
          data={items}
          keyExtractor={keyExtractor}
          renderItem={renderItem}
          recycleItems
          style={{ flex: 1, width: contentWidth }}
          contentContainerStyle={{
            paddingTop: 12,
            paddingBottom: bottomInset + (selectedLines.size > 0 ? 80 : 24),
            paddingLeft: 4,
          }}
        />
      </ScrollView>

      {/* Floating Action Pill when lines are selected */}
      {selectedLines.size > 0 && (
        <View
          style={{
            position: "absolute",
            bottom: bottomInset + 16,
            left: 20,
            right: 20,
            alignItems: "center",
          }}
        >
          <View className="flex-row items-center gap-3 px-4 py-2.5 rounded-full bg-card border border-border shadow-xl">
            <Text className="text-xs font-semibold text-foreground">
              {selectedLines.size} {selectedLines.size === 1 ? "line" : "lines"} selected
            </Text>

            <Pressable
              onPress={handleCopySelected}
              className="flex-row items-center gap-1.5 px-3 py-1.5 rounded-full bg-foreground active:opacity-80"
            >
              {copiedSelected ? (
                <>
                  <Check size={13} color="#000000" />
                  <Text className="text-xs font-bold text-black">Copied!</Text>
                </>
              ) : (
                <>
                  <Copy size={13} color="#000000" />
                  <Text className="text-xs font-bold text-black">Copy</Text>
                </>
              )}
            </Pressable>

            <Pressable
              onPress={handleClearSelection}
              hitSlop={8}
              className="p-1 rounded-full active:bg-white/10"
            >
              <X size={15} color={theme.colors.text.muted} />
            </Pressable>
          </View>
        </View>
      )}
    </View>
  );
});
