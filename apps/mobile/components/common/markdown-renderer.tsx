import React, { useState } from "react";
import { Linking, Pressable, Text, View } from "react-native";
import Markdown, { type ASTNode } from "react-native-markdown-display";
import { Check, Copy, FileCode } from "lucide-react-native";
import { theme } from "../../styles/theme";
import { setStringAsync } from "expo-clipboard";

interface MarkdownRendererProps {
  content: string;
}

/** Extension of ASTNode with the fields tokensToAST actually attaches. */
interface RenderNode extends ASTNode {
  sourceInfo?: string;
}

function CodeBlockHeader({
  language,
  code,
}: {
  language: string;
  code: string;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await setStringAsync(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <View className="flex-row items-center justify-between px-3.5 py-2 border-b border-border/60 bg-card-alt/40">
      <View className="flex-row items-center gap-1.5">
        <FileCode size={13} color={theme.colors.text.secondary} />
        <Text className="text-[11px] font-mono font-semibold text-foreground-secondary">
          {language || "code"}
        </Text>
      </View>
      <Pressable onPress={handleCopy} hitSlop={8}>
        {copied ? (
          <Check size={14} color={theme.colors.status.ready} />
        ) : (
          <Copy size={14} color={theme.colors.text.secondary} />
        )}
      </Pressable>
    </View>
  );
}

export function MarkdownRenderer({ content }: MarkdownRendererProps) {
  if (!content) return null;

  return (
    <Markdown
      style={{
        body: { color: "#ffffff" },
        text: { color: "#ffffff" },
        paragraph: { color: "#ffffff" },
        blockquote: {
          backgroundColor: "#16171a",
          borderLeftColor: "rgba(255, 255, 255, 0.2)",
          borderLeftWidth: 3,
          paddingHorizontal: 12,
          paddingVertical: 8,
          marginVertical: 6,
          borderRadius: 6,
        },
        code_inline: {
          backgroundColor: "#1c1d22",
          borderColor: "rgba(255, 255, 255, 0.15)",
          color: "#fdba74",
        },
        code_block: {
          backgroundColor: "#101113",
          borderColor: "rgba(255, 255, 255, 0.1)",
        },
        fence: {
          backgroundColor: "#101113",
          borderColor: "rgba(255, 255, 255, 0.1)",
        },
        list_item: {
          color: "#e4e4e7",
        },
        bullet_list: {
          color: "#e4e4e7",
        },
        ordered_list: {
          color: "#e4e4e7",
        },
      }}
      rules={{
        body: (node, children) => (
          <View key={node.key} className="w-full">
            {children}
          </View>
        ),
        paragraph: (node, children) => (
          <Text key={node.key} className="text-foreground text-[15px] leading-[23px] my-[3px]">
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
          <Text key={node.key} className="text-foreground text-[15px] font-bold mt-2.5 mb-1">
            {children}
          </Text>
        ),
        heading4: (node, children) => (
          <Text key={node.key} className="text-foreground text-sm font-bold mt-2 mb-0.5">
            {children}
          </Text>
        ),
        bullet_list: (node, children) => (
          <View key={node.key} className="my-1">
            {children}
          </View>
        ),
        ordered_list: (node, children) => (
          <View key={node.key} className="my-1">
            {children}
          </View>
        ),
        list_item: (node, children, parent) => {
          const isOrdered = parent && parent[0]?.type === "ordered_list";
          const bullet = isOrdered ? (node.index !== undefined ? `${node.index + 1}.` : "•") : "•";
          return (
            <View key={node.key} className="flex-row items-start my-[3px]">
              <Text className="text-foreground/70 mr-2.5 mt-0.5 text-[14px] font-semibold">
                {bullet}
              </Text>
              <View className="flex-1">{children}</View>
            </View>
          );
        },
        link: (node: RenderNode, children) => (
          <Text
            key={node.key}
            className="underline text-[15px]"
            style={{ color: "#7da7ff" }}
            onPress={() => {
              const href = node.attributes?.href;
              if (typeof href === "string") {
                Linking.openURL(href).catch(() => {});
              }
            }}
          >
            {children}
          </Text>
        ),
        code_inline: (node) => (
          <Text
            key={node.key}
            className="font-mono text-[12.5px] text-orange-300 px-1.5 py-0.5 rounded-md"
            style={{ backgroundColor: "rgba(255,255,255,0.08)" }}
          >
            {node.content}
          </Text>
        ),
        fence: (node: RenderNode) => {
          const codeText = node.content ? node.content.replace(/\n$/, "") : "";
          const language = node.sourceInfo ? node.sourceInfo.trim() : "";
          return (
            <View
              key={node.key}
              className="my-2.5 rounded-xl overflow-hidden"
              style={{ backgroundColor: "#101113", borderWidth: 1, borderColor: "rgba(255,255,255,0.1)" }}
            >
              <CodeBlockHeader language={language} code={codeText} />
              <View className="px-3.5 py-3">
                <Text className="text-foreground font-mono text-[12.5px] leading-5" selectable>
                  {codeText}
                </Text>
              </View>
            </View>
          );
        },
        code_block: (node: RenderNode) => {
          const codeText = node.content ? node.content.replace(/\n$/, "") : "";
          const language = node.sourceInfo ? node.sourceInfo.trim() : "";
          return (
            <View
              key={node.key}
              className="my-2.5 rounded-xl overflow-hidden"
              style={{ backgroundColor: "#101113", borderWidth: 1, borderColor: "rgba(255,255,255,0.1)" }}
            >
              <CodeBlockHeader language={language} code={codeText} />
              <View className="px-3.5 py-3">
                <Text className="text-foreground font-mono text-[12.5px] leading-5" selectable>
                  {codeText}
                </Text>
              </View>
            </View>
          );
        },
        strong: (node, children) => (
          <Text key={node.key} className="font-bold text-foreground">
            {children}
          </Text>
        ),
        em: (node, children) => (
          <Text key={node.key} className="italic text-foreground/90">
            {children}
          </Text>
        ),
        blockquote: (node, children) => (
          <View
            key={node.key}
            className="border-l-[3px] pl-3.5 my-2.5 rounded-r-xl py-2 pr-3"
            style={{
              borderColor: "rgba(255, 255, 255, 0.25)",
              backgroundColor: "rgba(255, 255, 255, 0.05)",
            }}
          >
            {children}
          </View>
        ),
        block_quote: (node, children) => (
          <View
            key={node.key}
            className="border-l-[3px] pl-3.5 my-2.5 rounded-r-xl py-2 pr-3"
            style={{
              borderColor: "rgba(255, 255, 255, 0.25)",
              backgroundColor: "rgba(255, 255, 255, 0.05)",
            }}
          >
            {children}
          </View>
        ),
        hr: (node) => (
          <View key={node.key} className="my-4" style={{ height: 1, backgroundColor: "rgba(255,255,255,0.1)" }} />
        ),
      }}
    >
      {content}
    </Markdown>
  );
}