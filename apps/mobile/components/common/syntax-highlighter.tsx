import React, { memo, useMemo } from "react";
import { View, Text, ScrollView } from "react-native";
import Prism from "prismjs";

// Load primary/core language grammars statically
import "prismjs/components/prism-clike";
import "prismjs/components/prism-javascript";
import "prismjs/components/prism-typescript";
import "prismjs/components/prism-jsx";
import "prismjs/components/prism-tsx";
import "prismjs/components/prism-python";
import "prismjs/components/prism-bash";
import "prismjs/components/prism-json";

// Extended language loaders — loaded on demand when encountered in code blocks
const EXTENDED_GRAMMAR_LOADERS: Record<string, () => void> = {
  rust: () => require("prismjs/components/prism-rust"),
  go: () => require("prismjs/components/prism-go"),
  c: () => require("prismjs/components/prism-c"),
  cpp: () => require("prismjs/components/prism-cpp"),
  csharp: () => require("prismjs/components/prism-csharp"),
  java: () => require("prismjs/components/prism-java"),
  kotlin: () => require("prismjs/components/prism-kotlin"),
  swift: () => require("prismjs/components/prism-swift"),
  sql: () => require("prismjs/components/prism-sql"),
  yaml: () => require("prismjs/components/prism-yaml"),
  markdown: () => require("prismjs/components/prism-markdown"),
  ruby: () => require("prismjs/components/prism-ruby"),
  php: () => require("prismjs/components/prism-php"),
  docker: () => require("prismjs/components/prism-docker"),
  css: () => require("prismjs/components/prism-css"),
  markup: () => require("prismjs/components/prism-markup"),
};

function getOrLoadGrammar(lang: string) {
  if (Prism.languages[lang]) return Prism.languages[lang];
  const loader = EXTENDED_GRAMMAR_LOADERS[lang];
  if (loader) {
    try {
      loader();
    } catch {
      // Ignore if loader fails
    }
  }
  return Prism.languages[lang];
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
