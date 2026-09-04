import { createHighlighterCore, type HighlighterCore, type ThemedToken } from "@shikijs/core";
import { createNativeEngine, isNativeEngineAvailable } from "react-native-shiki-engine";
import { createJavaScriptRegexEngine } from "@shikijs/engine-javascript";

import langTypescript from "@shikijs/langs/typescript";
import langTsx from "@shikijs/langs/tsx";
import langJavascript from "@shikijs/langs/javascript";
import langJsx from "@shikijs/langs/jsx";
import langRust from "@shikijs/langs/rust";
import langPython from "@shikijs/langs/python";
import langGo from "@shikijs/langs/go";
import langBash from "@shikijs/langs/bash";
import langJson from "@shikijs/langs/json";
import langYaml from "@shikijs/langs/yaml";
import langMarkdown from "@shikijs/langs/markdown";
import langCss from "@shikijs/langs/css";
import langHtml from "@shikijs/langs/html";
import langSql from "@shikijs/langs/sql";
import langDockerfile from "@shikijs/langs/dockerfile";
import langC from "@shikijs/langs/c";
import langCpp from "@shikijs/langs/cpp";
import langCsharp from "@shikijs/langs/csharp";
import langJava from "@shikijs/langs/java";
import langKotlin from "@shikijs/langs/kotlin";
import langSwift from "@shikijs/langs/swift";
import langRuby from "@shikijs/langs/ruby";
import themeVitesseDark from "@shikijs/themes/vitesse-dark";

import { getFileTypeLanguage } from "@/utils/icons/file-type-mapping";
import { useEffect, useState, useMemo } from "react";

export type { ThemedToken };

export const THEME_NAME = "vitesse-dark";

const SUPPORTED_LANGS = [
  langTypescript,
  langTsx,
  langJavascript,
  langJsx,
  langRust,
  langPython,
  langGo,
  langBash,
  langJson,
  langYaml,
  langMarkdown,
  langCss,
  langHtml,
  langSql,
  langDockerfile,
  langC,
  langCpp,
  langCsharp,
  langJava,
  langKotlin,
  langSwift,
  langRuby,
];

const CANONICAL_LANG_MAP: Record<string, string> = {
  ts: "typescript",
  typescript: "typescript",
  tsx: "tsx",
  js: "javascript",
  javascript: "javascript",
  jsx: "jsx",
  rs: "rust",
  rust: "rust",
  py: "python",
  python: "python",
  go: "go",
  golang: "go",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  shell: "bash",
  json: "json",
  jsonc: "json",
  yml: "yaml",
  yaml: "yaml",
  md: "markdown",
  markdown: "markdown",
  css: "css",
  html: "html",
  htm: "html",
  markup: "html",
  xml: "html",
  svg: "html",
  sql: "sql",
  docker: "dockerfile",
  dockerfile: "dockerfile",
  c: "c",
  cpp: "cpp",
  "c++": "cpp",
  cs: "csharp",
  csharp: "csharp",
  java: "java",
  kt: "kotlin",
  kotlin: "kotlin",
  swift: "swift",
  rb: "ruby",
  ruby: "ruby",
};

let highlighterInstance: HighlighterCore | null = null;
let highlighterPromise: Promise<HighlighterCore> | null = null;

export function resolveLanguage(filePathOrLang?: string): string {
  if (!filePathOrLang) return "";
  const rawLang = filePathOrLang.includes("/") || filePathOrLang.includes(".")
    ? getFileTypeLanguage(filePathOrLang)
    : filePathOrLang.trim().toLowerCase();

  return CANONICAL_LANG_MAP[rawLang] || rawLang;
}

/**
 * Initializes or returns the singleton Shiki highlighter instance.
 * Uses native C++ Oniguruma engine via JSI when available, falling back
 * to pure JS regex engine on web / dev.
 */
export async function getHighlighter(): Promise<HighlighterCore> {
  if (highlighterInstance) return highlighterInstance;
  if (highlighterPromise) return highlighterPromise;

  highlighterPromise = (async () => {
    try {
      const isNative = typeof isNativeEngineAvailable === "function" && isNativeEngineAvailable();
      const engine = isNative ? createNativeEngine() : createJavaScriptRegexEngine();

      const instance = await createHighlighterCore({
        langs: SUPPORTED_LANGS,
        themes: [themeVitesseDark],
        engine,
      });

      highlighterInstance = instance;
      return instance;
    } catch (err) {
      // Fallback with JS engine if native initialization throws
      const instance = await createHighlighterCore({
        langs: SUPPORTED_LANGS,
        themes: [themeVitesseDark],
        engine: createJavaScriptRegexEngine(),
      });
      highlighterInstance = instance;
      return instance;
    }
  })();

  return highlighterPromise;
}

// Eagerly initiate background loading
void getHighlighter().catch(() => {});

/**
 * Convert plain text lines into untokenized ThemedToken[][] for instant first paint.
 */
export function createPlainTokens(code: string): ThemedToken[][] {
  if (!code) return [];
  const lines = code.split("\n");
  return lines.map((line) => [
    {
      content: line || " ",
      color: "#e4e4e7",
      offset: 0,
    },
  ]);
}

/**
 * Synchronous tokenization if highlighter is already initialized.
 * Returns null if highlighter is still loading.
 */
export function tokenizeSync(code: string, langName: string): ThemedToken[][] | null {
  if (!highlighterInstance || !code) return null;
  const lang = resolveLanguage(langName);
  const loadedLangs = highlighterInstance.getLoadedLanguages();
  if (!lang || !loadedLangs.includes(lang)) {
    return createPlainTokens(code);
  }

  try {
    return highlighterInstance.codeToTokensBase(code, {
      lang,
      theme: THEME_NAME,
    });
  } catch {
    return createPlainTokens(code);
  }
}

/**
 * React hook that tokenizes code using Shiki.
 * Returns plain tokens instantly, and updates seamlessly once Shiki finishes tokenizing.
 */
export function useShikiTokens(code: string, filePathOrLang?: string): ThemedToken[][] {
  const lang = useMemo(() => resolveLanguage(filePathOrLang), [filePathOrLang]);

  // Try immediate synchronous tokenization if highlighter is already ready
  const initialTokens = useMemo(() => {
    return tokenizeSync(code, lang) || createPlainTokens(code);
  }, [code, lang]);

  const [tokens, setTokens] = useState<ThemedToken[][]>(initialTokens);

  useEffect(() => {
    let cancelled = false;

    // Check if synchronous tokenization worked
    const syncRes = tokenizeSync(code, lang);
    if (syncRes) {
      setTokens(syncRes);
      return;
    }

    // Otherwise await highlighter ready and tokenize
    getHighlighter().then((highlighter) => {
      if (cancelled) return;
      const loadedLangs = highlighter.getLoadedLanguages();
      if (!lang || !loadedLangs.includes(lang)) {
        setTokens(createPlainTokens(code));
        return;
      }
      try {
        const result = highlighter.codeToTokensBase(code, {
          lang,
          theme: THEME_NAME,
        });
        if (!cancelled) setTokens(result);
      } catch {
        if (!cancelled) setTokens(createPlainTokens(code));
      }
    });

    return () => {
      cancelled = true;
    };
  }, [code, lang]);

  return tokens;
}
