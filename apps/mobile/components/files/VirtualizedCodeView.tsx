import React, { memo, useCallback, useMemo } from "react";
import { View, Text, ScrollView, Dimensions } from "react-native";
import { LegendList } from "@legendapp/list/react-native";
import * as Clipboard from "expo-clipboard";
import type { ThemedToken } from "@/services/highlighter";
import { theme } from "@/styles/theme";

interface LineItem {
  lineNumber: number;
  tokens: ThemedToken[];
}

const LINE_HEIGHT = 18;
const CHAR_WIDTH_APPROX = 6.8;

const CodeLine = memo(function CodeLine({
  lineNumber,
  tokens,
  gutterWidth,
}: {
  lineNumber: number;
  tokens: ThemedToken[];
  gutterWidth: number;
}) {
  const onLongPressLine = useCallback(() => {
    const fullText = tokens.map((t) => t.content).join("");
    void Clipboard.setStringAsync(fullText);
  }, [tokens]);

  return (
    <View
      style={{
        flexDirection: "row",
        height: LINE_HEIGHT,
        alignItems: "center",
      }}
    >
      {/* Gutter Line Number */}
      <View
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
            fontFamily: "monospace",
            color: "rgba(255, 255, 255, 0.28)",
          }}
        >
          {lineNumber}
        </Text>
      </View>

      {/* Code Text */}
      <Text
        onLongPress={onLongPressLine}
        style={{
          fontSize: 11,
          lineHeight: LINE_HEIGHT,
          fontFamily: "monospace",
          color: theme.colors.text.primary,
        }}
      >
        {tokens.map((token, j) => (
          <Text
            key={j}
            style={{
              color: token.color || theme.colors.text.primary,
              fontStyle: token.fontStyle === 1 ? "italic" : "normal",
            }}
          >
            {token.content}
          </Text>
        ))}
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

  const totalLines = tokens.length;
  // Compute gutter width dynamically based on number of digits (e.g. 2 digits = 28px, 4 digits = 44px)
  const digits = String(totalLines || 1).length;
  const gutterWidth = Math.max(28, digits * 8 + 14);

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

  const renderItem = useCallback(
    ({ item }: { item: LineItem }) => {
      return (
        <CodeLine
          lineNumber={item.lineNumber}
          tokens={item.tokens}
          gutterWidth={gutterWidth}
        />
      );
    },
    [gutterWidth],
  );

  const keyExtractor = useCallback((item: LineItem) => `line-${item.lineNumber}`, []);

  return (
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
          paddingBottom: bottomInset + 24,
          paddingLeft: 4,
        }}
      />
    </ScrollView>
  );
});
