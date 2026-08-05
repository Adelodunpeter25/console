import React from "react";
import { Text, View, StyleSheet } from "react-native";
import { theme } from "../../styles/theme";

interface MarkdownRendererProps {
  content: string;
}

export function MarkdownRenderer({ content }: MarkdownRendererProps) {
  if (!content) return null;

  // Split into blocks: code blocks vs normal markdown text
  const parts = content.split(/(```[\s\S]*?```)/g);

  return (
    <View style={styles.container}>
      {parts.map((part, index) => {
        if (part.startsWith("```") && part.endsWith("```")) {
          // Extract code content and optional language specifier
          const lines = part.slice(3, -3).trim().split("\n");
          let language = "";
          let codeText = lines.join("\n");
          
          if (lines[0] && lines[0].length < 15 && !lines[0].includes(" ") && !lines[0].includes("(") && !lines[0].includes("=")){
            language = lines[0];
            codeText = lines.slice(1).join("\n");
          }

          return (
            <View key={index} style={styles.codeBlock}>
              {language ? <Text style={styles.codeLang}>{language.toUpperCase()}</Text> : null}
              <Text style={styles.codeText} selectable>{codeText}</Text>
            </View>
          );
        }

        // Render normal text block with inline parsing
        return (
          <View key={index} style={styles.textBlock}>
            {part.split("\n").map((line, lineIdx) => {
              const trimmed = line.trim();
              if (!trimmed) return null;

              // Render Headings
              if (trimmed.startsWith("#")) {
                const headingLevel = (trimmed.match(/^#+/) || [""])[0].length;
                const headingText = trimmed.replace(/^#+\s*/, "");
                const headingStyle = headingLevel === 1 ? styles.h1 : headingLevel === 2 ? styles.h2 : styles.h3;
                return (
                  <Text key={lineIdx} style={headingStyle} selectable>
                    {headingText}
                  </Text>
                );
              }

              // Render bullet points
              if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
                const bulletText = trimmed.slice(2);
                return (
                  <View key={lineIdx} style={styles.bulletRow}>
                    <Text style={styles.bulletPoint}>•</Text>
                    <Text style={styles.bulletText} selectable>
                      {renderInlineFormatting(bulletText)}
                    </Text>
                  </View>
                );
              }

              // Standard Paragraph
              return (
                <Text key={lineIdx} style={styles.paragraph} selectable>
                  {renderInlineFormatting(trimmed)}
                </Text>
              );
            })}
          </View>
        );
      })}
    </View>
  );
}

// Simple inline parser for bold (**text**), italics (*text*), and inline code (`code`)
function renderInlineFormatting(text: string) {
  const parts = text.split(/(\*\*.*?\*\*|\*.*?\*|`.*?`)/g);
  return parts.map((part, index) => {
    if (part.startsWith("**") && part.endsWith("**")) {
      return (
        <Text key={index} style={styles.bold}>
          {part.slice(2, -2)}
        </Text>
      );
    }
    if (part.startsWith("*") && part.endsWith("*")) {
      return (
        <Text key={index} style={styles.italic}>
          {part.slice(1, -1)}
        </Text>
      );
    }
    if (part.startsWith("`") && part.endsWith("`")) {
      return (
        <Text key={index} style={styles.inlineCode}>
          {part.slice(1, -1)}
        </Text>
      );
    }
    return part;
  });
}

const styles = StyleSheet.create({
  container: {
    width: "100%",
  },
  textBlock: {
    marginVertical: 4,
  },
  paragraph: {
    color: theme.colors.text.primary,
    fontSize: 14,
    lineHeight: 22,
    marginVertical: 4,
  },
  bold: {
    fontWeight: "bold",
  },
  italic: {
    fontStyle: "italic",
  },
  inlineCode: {
    fontFamily: theme.fonts.mono,
    fontSize: 12,
    backgroundColor: theme.colors.surface,
    color: theme.colors.status.running,
    paddingHorizontal: 4,
    paddingVertical: 2,
    borderRadius: 4,
  },
  codeBlock: {
    backgroundColor: theme.colors.surface,
    borderColor: theme.colors.border,
    borderWidth: 1,
    borderRadius: theme.roundness.md,
    padding: 12,
    marginVertical: 8,
  },
  codeLang: {
    color: theme.colors.text.muted,
    fontSize: 10,
    fontFamily: theme.fonts.monoBold,
    marginBottom: 6,
    letterSpacing: 1,
  },
  codeText: {
    color: theme.colors.text.primary,
    fontFamily: theme.fonts.mono,
    fontSize: 12,
    lineHeight: 18,
  },
  h1: {
    color: theme.colors.text.primary,
    fontSize: 20,
    fontWeight: "bold",
    marginTop: 16,
    marginBottom: 8,
  },
  h2: {
    color: theme.colors.text.primary,
    fontSize: 18,
    fontWeight: "bold",
    marginTop: 12,
    marginBottom: 6,
  },
  h3: {
    color: theme.colors.text.primary,
    fontSize: 16,
    fontWeight: "bold",
    marginTop: 8,
    marginBottom: 4,
  },
  bulletRow: {
    flexDirection: "row",
    alignItems: "flex-start",
    marginVertical: 3,
  },
  bulletPoint: {
    color: theme.colors.text.muted,
    fontSize: 14,
    marginRight: 6,
    lineHeight: 20,
  },
  bulletText: {
    flex: 1,
    color: theme.colors.text.primary,
    fontSize: 14,
    lineHeight: 20,
  },
});
