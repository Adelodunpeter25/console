import React, { memo, useMemo } from "react";
import { View, Text, ScrollView } from "react-native";
import Prism from "prismjs";

// Load primary and extended language grammars statically in strict dependency order
import "prismjs/components/prism-clike";
import "prismjs/components/prism-markup";
import "prismjs/components/prism-javascript";
import "prismjs/components/prism-typescript";
import "prismjs/components/prism-jsx";
import "prismjs/components/prism-tsx";
import "prismjs/components/prism-c";
import "prismjs/components/prism-cpp";
import "prismjs/components/prism-csharp";
import "prismjs/components/prism-java";
import "prismjs/components/prism-kotlin";
import "prismjs/components/prism-rust";
import "prismjs/components/prism-go";
import "prismjs/components/prism-swift";
import "prismjs/components/prism-python";
import "prismjs/components/prism-bash";
import "prismjs/components/prism-json";
import "prismjs/components/prism-yaml";
import "prismjs/components/prism-sql";
import "prismjs/components/prism-markdown";
import "prismjs/components/prism-ruby";
import "prismjs/components/prism-docker";
import "prismjs/components/prism-css";

import { getFileTypeLanguage } from "@/utils/icons/file-type-mapping";

function getOrLoadGrammar(lang: string) {
  if (!lang) return null;
  return Prism.languages[lang] ?? null;
}

interface SyntaxHighlighterProps {
  code: string;
  language?: string;
}

const LANGUAGE_ALIASES: Record<string, string> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  py: "python",
  python3: "python",
  rs: "rust",
  golang: "go",
  sh: "bash",
  zsh: "bash",
  shell: "bash",
  yml: "yaml",
  md: "markdown",
  dockerfile: "docker",
  cs: "csharp",
  rb: "ruby",
  kt: "kotlin",
  htm: "markup",
  html: "markup",
  xml: "markup",
  svg: "markup",
};

/**
 * Resolves a file path to a canonical language id for highlighting.
 * Delegates to the shared file-type mapping in utils/icons.
 */
export function getLanguageFromPath(filePath?: string): string {
  if (!filePath) return "";
  return getFileTypeLanguage(filePath);
}

export function renderHighlightedLine(
  line: string,
  language = "",
  keyPrefix = "l",
): React.ReactNode {
  if (!line) return " ";
  const normalizedLang = language.trim().toLowerCase();
  const canonicalLang = LANGUAGE_ALIASES[normalizedLang] || normalizedLang;
  const grammar = getOrLoadGrammar(canonicalLang);

  if (!grammar) {
    return line;
  }

  try {
    const tokens = Prism.tokenize(line, grammar);
    return renderPrismTokens(tokens, keyPrefix);
  } catch {
    return line;
  }
}

interface TokenStyle {
  color: string;
  fontStyle?: "italic" | "normal";
  fontWeight?: "bold" | "normal" | "600";
}

const TOKEN_STYLES: Record<string, TokenStyle> = {
  keyword: { color: "#c084fc", fontWeight: "600" }, // vibrant purple (for, let, const, fn, return, if, else)
  builtin: { color: "#38bdf8" }, // sky-400 (console, print, len, Array)
  "class-name": { color: "#facc15", fontWeight: "600" }, // yellow-400 (Promise, Result, Option, Class)
  type: { color: "#facc15" }, // yellow-400 (string, number, boolean, i32, void)
  string: { color: "#4ade80" }, // emerald-400 ("hello", 'world', `template`)
  char: { color: "#4ade80" },
  "template-string": { color: "#4ade80" },
  number: { color: "#fb923c" }, // orange-400 (0, 42, 3.14, 0xff)
  boolean: { color: "#f87171" }, // red-400 (true, false, null, undefined)
  function: { color: "#60a5fa" }, // blue-400 (log, println, fmt, fetch)
  "function-variable": { color: "#60a5fa" },
  method: { color: "#60a5fa" },
  comment: { color: "#71717a", fontStyle: "italic" }, // muted zinc (// ..., /* ... */, # ...)
  prolog: { color: "#71717a", fontStyle: "italic" },
  doctype: { color: "#71717a", fontStyle: "italic" },
  cdata: { color: "#71717a", fontStyle: "italic" },
  operator: { color: "#38bdf8" }, // sky-400 (+, -, =, :=, =>, <=, ||)
  punctuation: { color: "#a1a1aa" }, // light gray ({, }, (, ), [, ], ;, :)
  property: { color: "#f472b6" }, // pink-400 (key: value, obj.prop)
  constant: { color: "#fb923c" }, // orange-400
  symbol: { color: "#f472b6" },
  tag: { color: "#f87171" }, // red-400 (<div>, <span>)
  "attr-name": { color: "#93c5fd" }, // blue-300 (className, href, id)
  "attr-value": { color: "#4ade80" }, // emerald-400
  regex: { color: "#34d399" }, // teal-400
  important: { color: "#f87171", fontWeight: "bold" },
  variable: { color: "#e4e4e7" },
};

function renderPrismTokens(
  tokens: Array<string | Prism.Token>,
  keyPrefix = "t",
): React.ReactNode[] {
  return tokens.map((token, index) => {
    const key = `${keyPrefix}-${index}`;

    if (typeof token === "string") {
      return (
        <Text key={key} style={{ color: "#e4e4e7" }}>
          {token}
        </Text>
      );
    }

    const tokenStyle = TOKEN_STYLES[token.type] ?? { color: "#e4e4e7" };

    if (typeof token.content === "string") {
      return (
        <Text
          key={key}
          style={{
            color: tokenStyle.color,
            fontStyle: tokenStyle.fontStyle ?? "normal",
            fontWeight: tokenStyle.fontWeight ?? "normal",
          }}
        >
          {token.content}
        </Text>
      );
    }

    if (Array.isArray(token.content)) {
      return (
        <Text
          key={key}
          style={{
            color: tokenStyle.color,
            fontStyle: tokenStyle.fontStyle ?? "normal",
            fontWeight: tokenStyle.fontWeight ?? "normal",
          }}
        >
          {renderPrismTokens(token.content as Array<string | Prism.Token>, `${key}-sub`)}
        </Text>
      );
    }

    return (
      <Text
        key={key}
        style={{
          color: tokenStyle.color,
          fontStyle: tokenStyle.fontStyle ?? "normal",
          fontWeight: tokenStyle.fontWeight ?? "normal",
        }}
      >
        {String(token.content)}
      </Text>
    );
  });
}

export const SyntaxHighlighter = memo(function SyntaxHighlighter({
  code,
  language = "",
}: SyntaxHighlighterProps) {
  const normalizedLang = language.trim().toLowerCase();
  const canonicalLang = LANGUAGE_ALIASES[normalizedLang] || normalizedLang;
  const grammar = getOrLoadGrammar(canonicalLang);

  const tokenElements = useMemo(() => {
    if (!code) return null;

    if (!grammar) {
      // Fallback to plain text if no grammar found
      return (
        <Text style={{ color: "#e4e4e7" }}>
          {code}
        </Text>
      );
    }

    try {
      const tokens = Prism.tokenize(code, grammar);
      return renderPrismTokens(tokens);
    } catch {
      return (
        <Text style={{ color: "#e4e4e7" }}>
          {code}
        </Text>
      );
    }
  }, [code, grammar]);

  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false}>
      <Text
        className="font-mono text-[12.5px] leading-5"
        selectable
      >
        {tokenElements}
      </Text>
    </ScrollView>
  );
});
