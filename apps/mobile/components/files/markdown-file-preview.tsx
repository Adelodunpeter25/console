import React from "react";
import { ScrollView, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { MarkdownRenderer } from "@/components/common/markdown-renderer";

interface MarkdownFilePreviewProps {
  content: string;
}

export function MarkdownFilePreview({ content }: MarkdownFilePreviewProps) {
  const insets = useSafeAreaInsets();
  return (
    <ScrollView
      className="flex-1 bg-screen"
      contentContainerStyle={{ padding: 16, paddingBottom: insets.bottom + 16 }}
      showsVerticalScrollIndicator={false}
    >
      <View className="flex-1">
        <MarkdownRenderer content={content} />
      </View>
    </ScrollView>
  );
}

export function isMarkdownPath(path: string | undefined): boolean {
  if (!path) return false;
  const lower = path.toLowerCase();
  return lower.endsWith(".md") || lower.endsWith(".mdx") || lower.endsWith(".markdown") || lower.endsWith(".mkd");
}
