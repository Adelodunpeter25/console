import React from "react";
import { Brain, ChevronDown, ChevronRight } from "lucide-react";

interface ThinkingBlockProps {
  text: string;
  defaultExpanded?: boolean;
  isStreaming?: boolean;
}

/**
 * Purple-tinted collapsible panel for displaying agent reasoning / thinking content.
 * Collapsed by default with a clean expand/collapse header toggle.
 */
export function ThinkingBlock({
  text,
  defaultExpanded = false,
  isStreaming = false,
}: ThinkingBlockProps) {
  const [isExpanded, setIsExpanded] = React.useState(defaultExpanded || isStreaming);

  const wordCount = React.useMemo(
    () => (text ? text.trim().split(/\s+/).filter(Boolean).length : 0),
    [text],
  );

  if (!text) return null;

  return (
    <div className="rounded-lg bg-thinking/80 border border-thinking-border overflow-hidden transition-all">
      {/* Collapsible Header */}
      <button
        onClick={() => setIsExpanded((prev) => !prev)}
        className="w-full flex items-center justify-between px-3.5 py-2 hover:bg-purple-500/5 transition-colors cursor-pointer select-none"
      >
        <div className="flex items-center gap-2">
          <Brain size={14} className="text-purple-400 shrink-0" />
          <span className="text-xs font-semibold text-purple-400 uppercase tracking-wider">
            {isStreaming ? "Thinking..." : "Thought Process"}
          </span>
          {wordCount > 0 && (
            <span className="text-[11px] text-foreground-muted font-mono">
              ({wordCount} {wordCount === 1 ? "word" : "words"})
            </span>
          )}
        </div>

        <div className="flex items-center gap-1 text-foreground-muted hover:text-foreground transition-colors">
          <span className="text-[11px] text-foreground-muted">
            {isExpanded ? "Collapse" : "Expand"}
          </span>
          {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </div>
      </button>

      {/* Expanded Content */}
      {isExpanded && (
        <div className="px-3.5 pb-3 pt-1 border-t border-purple-500/15 bg-black/20">
          <p className="text-xs text-foreground-secondary italic whitespace-pre-wrap break-words selectable-text leading-relaxed font-mono">
            {text}
          </p>
        </div>
      )}
    </div>
  );
}
