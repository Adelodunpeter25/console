import React from "react";

interface UserBubbleProps {
  content: string;
}

/**
 * Amber-tinted right-aligned bubble for user messages.
 * Memoized so streaming tokens don't re-render already-sent messages.
 */
export const UserBubble = React.memo(function UserBubble({ content }: UserBubbleProps) {
  return (
    <div className="flex justify-end">
      <div className="max-w-[80%] rounded-2xl bg-user-bubble border border-user-bubble-border px-4 py-3">
        <p className="text-sm text-foreground whitespace-pre-wrap break-words selectable-text">{content}</p>
      </div>
    </div>
  );
});
