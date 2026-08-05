import React from "react";
import type { ImageAttachment } from "@console/types";
import { ImageViewerModal } from "../common";

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
  const [previewAttachment, setPreviewAttachment] = React.useState<ImageAttachment | null>(null);

  return (
    <div className="flex justify-end">
      <div className="max-w-[80%] rounded-2xl bg-user-bubble border border-user-bubble-border px-4 py-3">
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-2">
            {attachments.map((att, i) => (
              <button
                key={i}
                type="button"
                onClick={() => setPreviewAttachment(att)}
                className="h-16 w-16 overflow-hidden rounded-lg border border-user-bubble-border"
                title={`Preview attachment ${i + 1}`}
              >
                <img
                  src={`data:${att.mimeType};base64,${att.data}`}
                  alt={`attachment ${i + 1}`}
                  className="h-full w-full object-cover"
                />
              </button>
            ))}
          </div>
        )}
        {content && (
          <p className="text-sm text-foreground whitespace-pre-wrap break-words selectable-text">
            {content}
          </p>
        )}
      </div>
      {previewAttachment && (
        <ImageViewerModal
          src={`data:${previewAttachment.mimeType};base64,${previewAttachment.data}`}
          alt="Attachment preview"
          onClose={() => setPreviewAttachment(null)}
        />
      )}
    </div>
  );
});
