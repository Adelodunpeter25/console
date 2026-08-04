import React from "react";
import type { ImageAttachment } from "@console/types";

interface UserBubbleProps {
  content: string;
  attachments?: ImageAttachment[];
}

/**
 * Amber-tinted right-aligned bubble for user messages.
 * Memoized so streaming tokens don't re-render already-sent messages.
 */
export const UserBubble = React.memo(function UserBubble({
  content,
  attachments = [],
}: UserBubbleProps) {
  return (
    <div className="flex justify-end">
      <div className="max-w-[80%] rounded-2xl bg-user-bubble border border-user-bubble-border px-4 py-3">
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-2">
            {attachments.map((att, i) => (
              <img
                key={i}
                src={`data:${att.mimeType};base64,${att.data}`}
                alt={`attachment ${i + 1}`}
                className="max-h-40 max-w-full rounded-lg border border-user-bubble-border"
              />
            ))}
          </div>
        )}
        {content && (
          <p className="text-sm text-foreground whitespace-pre-wrap break-words selectable-text">
            {content}
          </p>
        )}
      </div>
    </div>
  );
});
