import React from "react";
import { Streamdown } from "streamdown";

interface MarkdownRendererProps {
  content: string;
  className?: string;
  /** When true, animates newly-streamed content and handles unterminated
      markdown blocks gracefully (code fences, tables) while tokens arrive. */
  streaming?: boolean;
}

/**
 * Renders markdown content with Streamdown — a streaming-first drop-in
 * replacement for react-markdown. Handles GFM, syntax highlighting (Shiki),
 * math, and mermaid out of the box, and parses incomplete markdown so live
 * streaming output doesn't flicker. Memoized internally.
 *
 * The `markdown-body` class is kept on Streamdown's root for the text-selection
 * re-enable rule in index.css (the app disables selection app-wide by default).
 */
export function MarkdownRenderer({ content, className, streaming }: MarkdownRendererProps) {
  return (
    <Streamdown
      className={[className, "markdown-body"].filter(Boolean).join(" ")}
      mode={streaming ? "streaming" : "static"}
      isAnimating={streaming}
    >
      {content}
    </Streamdown>
  );
}
