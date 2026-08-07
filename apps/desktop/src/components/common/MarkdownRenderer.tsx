import React from "react";
import { code } from "@streamdown/code";
import { Streamdown } from "streamdown";

interface MarkdownRendererProps {
  content: string;
  className?: string;
  /** When true, animates newly-streamed content and handles unterminated
      markdown blocks gracefully (code fences, tables) while tokens arrive. */
  streaming?: boolean;
}

const PLUGINS = { code };
const CONTROLS = { code: false };

/**
 * Renders markdown content with Streamdown — a streaming-first drop-in
 * replacement for react-markdown. Handles GFM, syntax highlighting (Shiki),
 * math, and mermaid out of the box, and parses incomplete markdown so live
 * streaming output doesn't flicker. Memoized internally.
 *
 * The `markdown-body` class is kept on Streamdown's root for the text-selection
 * re-enable rule in index.css (the app disables selection app-wide by default).
 *
 * Wrapped in React.memo with props hoisted to module-level constants so that
 * parent re-renders don't create new plugin/control objects that would defeat
 * Streamdown's internal memoization.
 */
export const MarkdownRenderer = React.memo(function MarkdownRenderer({
  content,
  className,
  streaming,
}: MarkdownRendererProps) {
  const resolvedClassName = React.useMemo(
    () => [className, "markdown-body"].filter(Boolean).join(" "),
    [className],
  );

  return (
    <Streamdown
      className={resolvedClassName}
      mode={streaming ? "streaming" : "static"}
      isAnimating={streaming}
      plugins={PLUGINS}
      controls={CONTROLS}
    >
      {content}
    </Streamdown>
  );
});
