import { Brain } from "lucide-react";

interface ThinkingBlockProps {
  text: string;
}

/**
 * Purple-tinted panel for displaying agent reasoning / thinking content.
 * Shared by AssistantBubble (persisted) and StreamingBubble (live).
 */
export function ThinkingBlock({ text }: ThinkingBlockProps) {
  return (
    <div className="rounded-lg bg-thinking border border-thinking-border px-3.5 py-2.5">
      <div className="flex items-center gap-1.5 mb-1">
        <Brain size={12} className="text-purple-400" />
        <span className="text-xs font-semibold text-purple-400 uppercase tracking-wider">
          Thinking
        </span>
      </div>
      <p className="text-sm text-foreground-secondary italic whitespace-pre-wrap break-words">
        {text}
      </p>
    </div>
  );
}
