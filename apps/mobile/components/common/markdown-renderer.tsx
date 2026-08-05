import React from "react";
import { View, Text } from "react-native";
import Markdown from "react-native-markdown-display";

interface MarkdownRendererProps {
  content: string;
}

export function MarkdownRenderer({ content }: MarkdownRendererProps) {
  if (!content) return null;

  return (
    <Markdown
      rules={{
        body: (node, children) => (
          <View key={node.key} className="w-full">
            {children}
          </View>
        ),
        paragraph: (node, children) => (
          <Text key={node.key} className="text-foreground text-sm leading-6 my-1">
            {children}
          </Text>
        ),
        heading1: (node, children) => (
          <Text key={node.key} className="text-foreground text-lg font-bold mt-4 mb-2">
            {children}
          </Text>
        ),
        heading2: (node, children) => (
          <Text key={node.key} className="text-foreground text-base font-bold mt-3 mb-1.5">
            {children}
          </Text>
        ),
        heading3: (node, children) => (
          <Text key={node.key} className="text-foreground text-sm font-bold mt-2.5 mb-1">
            {children}
          </Text>
        ),
        bullet_list: (node, children) => (
          <View key={node.key} className="my-1 pl-1">
            {children}
          </View>
        ),
        ordered_list: (node, children) => (
          <View key={node.key} className="my-1 pl-1">
            {children}
          </View>
        ),
        list_item: (node, children, parent) => {
          const isOrdered = parent && parent[0]?.type === "ordered_list";
          return (
            <View key={node.key} className="flex-row items-start my-0.5">
              <Text className="text-foreground-secondary mr-2 mt-0.5 text-xs">
                {isOrdered ? "•" : "•"}
              </Text>
              <View className="flex-1">{children}</View>
            </View>
          );
        },
        code_inline: (node) => (
          <Text key={node.key} className="font-mono text-xs bg-card-alt text-orange-400 px-1 py-0.5 rounded">
            {node.content}
          </Text>
        ),
        fence: (node) => {
          // Block code blocks
          const codeText = node.content ? node.content.trim() : "";
          return (
            <View key={node.key} className="bg-card border border-border rounded-xl p-3 my-2">
              <Text className="text-foreground font-mono text-xs leading-5" selectable>
                {codeText}
              </Text>
            </View>
          );
        },
        code_block: (node) => (
          <View key={node.key} className="bg-card border border-border rounded-xl p-3 my-2">
            <Text className="text-foreground font-mono text-xs leading-5" selectable>
              {node.content ? node.content.trim() : ""}
            </Text>
          </View>
        ),
        strong: (node, children) => (
          <Text key={node.key} className="font-bold">
            {children}
          </Text>
        ),
        em: (node, children) => (
          <Text key={node.key} className="italic">
            {children}
          </Text>
        ),
        block_quote: (node, children) => (
          <View key={node.key} className="border-l-2 border-border pl-3 my-2 italic">
            {children}
          </View>
        ),
        hr: (node) => (
          <View key={node.key} className="border-b border-border my-3" />
        ),
      }}
    >
      {content}
    </Markdown>
  );
}
