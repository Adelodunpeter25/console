import React from "react";
import type { ImageAttachment } from "@console/types";
import { ImageViewerModal } from "../common/ImageViewerModal";

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
  const attachmentKeyOccurrences = new Map<string, number>();

  return (
    <div className="flex justify-end">
      <div className="max-w-[80%] rounded-xl bg-user-bubble border border-user-bubble-border px-4 py-2">
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2 mb-2">
            {attachments.map((att, i) => {
              const base = `${att.mimeType}:${att.data}`;
              const occurrence = attachmentKeyOccurrences.get(base) ?? 0;
              attachmentKeyOccurrences.set(base, occurrence + 1);
              return (
                <button
                  key={`${base}:${occurrence}`}
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
              );
            })}
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
