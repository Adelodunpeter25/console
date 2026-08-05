import React from "react";
import { ArrowUp, Square, Paperclip, X } from "lucide-react";
import { toast } from "sonner";
import type { ApprovalMode, ImageAttachment, ProjectInfo } from "@console/types";
import {
  DragDropZone,
  ImageViewerModal,
  ModelSelector,
  ApprovalModeSelector,
  ProjectSelector,
} from "../common";
import { ComposerAutocomplete } from "./ComposerAutocomplete";

interface ComposerProps {
  value: string;
  onChange: (value: string) => void;
  onSend: () => void;
  onAbort: () => void;
  running: boolean;
  disabled: boolean;
  selectedModel: string | null;
  selectedModelSupportsImages?: boolean;
  onModelChange: (modelId: string) => void;
  approvalMode: ApprovalMode;
  onApprovalModeChange: (mode: ApprovalMode) => void;
  projects: ProjectInfo[];
  selectedProjectId: string | null;
  projectFallbackLabel: string;
  onProjectChange: (project: ProjectInfo) => void;
  /** Active session id — scopes slash-command + @-file autocomplete. */
  sessionId?: string | null;
  /** Pending image attachments to show as thumbnails above the textarea. */
  attachments?: ImageAttachment[];
  onPickImages?: () => void;
  onAddAttachments?: (attachments: ImageAttachment[]) => void;
  onRemoveAttachment?: (index: number) => void;
}

/**
 * Conductor-style composer: auto-growing textarea, send/stop button, and a
 * selector layer below it for model, approval mode, and working folder.
 * Supports / slash-command autocomplete and @ file references (FFF-powered).
 */
export function Composer({
  value,
  onChange,
  onSend,
  onAbort,
  running,
  disabled,
  selectedModel,
  selectedModelSupportsImages,
  onModelChange,
  approvalMode,
  onApprovalModeChange,
  projects,
  selectedProjectId,
  projectFallbackLabel,
  onProjectChange,
  sessionId,
  attachments = [],
  onPickImages,
  onAddAttachments,
  onRemoveAttachment,
}: ComposerProps) {
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const [previewAttachment, setPreviewAttachment] = React.useState<ImageAttachment | null>(null);
  const handleDropFiles = async (imageFiles: File[]) => {
    try {
      const droppedAttachments = await Promise.all(
        imageFiles.map(
          (file) =>
            new Promise<ImageAttachment>((resolve, reject) => {
              const reader = new FileReader();
              reader.onload = () => {
                const dataUrl = String(reader.result ?? "");
                const separator = dataUrl.indexOf(",");
                if (separator === -1) {
                  reject(new Error("Invalid image data"));
                  return;
                }
                resolve({ data: dataUrl.slice(separator + 1), mimeType: file.type });
              };
              reader.onerror = () => reject(reader.error ?? new Error("Unable to read image"));
              reader.readAsDataURL(file);
            }),
        ),
      );
      onAddAttachments?.(droppedAttachments);
    } catch {
      toast.error("Unable to add dropped image.");
    }
  };

  // Auto-grow textarea height up to a max.
  React.useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
      textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 160)}px`;
    }
  }, [value]);

  return (
    <>
      <div className="px-6 pb-4 pt-2">
      <div className="max-w-3xl mx-auto">
        <div className="relative">
          <ComposerAutocomplete
            value={value}
            sessionId={sessionId ?? null}
            onPick={onChange}
            textareaRef={textareaRef}
          />
          <DragDropZone
            className="bg-card border border-border rounded-2xl focus-within:border-border-strong"
            accept={(file) => file.type.startsWith("image/")}
            onDropFiles={handleDropFiles}
          >
            {attachments.length > 0 && (
              <div className="flex flex-wrap gap-2 px-3 pt-3">
                {attachments.map((att, i) => (
                  <div key={i} className="relative group">
                    <button
                      type="button"
                      onClick={() => setPreviewAttachment(att)}
                      className="block h-16 w-16 overflow-hidden rounded-lg border border-border"
                      title={`Preview attachment ${i + 1}`}
                    >
                      <img
                        src={`data:${att.mimeType};base64,${att.data}`}
                        alt={`attachment ${i + 1}`}
                        className="h-full w-full object-cover"
                      />
                    </button>
                    <button
                      onClick={() => onRemoveAttachment?.(i)}
                      className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-black/70 border border-border text-foreground-muted opacity-0 group-hover:opacity-100 focus-visible:opacity-100 hover:text-foreground flex items-center justify-center transition-opacity"
                      title="Remove"
                    >
                      <X size={11} />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <textarea
              ref={textareaRef}
              value={value}
              onChange={(e) => onChange(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  onSend();
                }
              }}
              placeholder="Ask agent to write code...  ( / for commands, @ for files )"
              rows={1}
              className="w-full bg-transparent px-4 pt-3.5 pb-2 text-sm text-foreground placeholder:text-foreground-muted outline-none resize-none"
              style={{ minHeight: "44px", maxHeight: "160px" }}
            />

            {/* Toolbar */}
            <div className="flex items-center gap-1 px-3 pb-2.5">
              <button
                disabled={selectedModelSupportsImages === false}
                className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-foreground-muted hover:text-foreground-secondary hover:bg-white/5 transition-colors disabled:cursor-not-allowed disabled:opacity-40"
                title={selectedModelSupportsImages === false ? "This model does not support images" : "Attach image(s)"}
                onClick={() => {
                  onPickImages?.();
                  textareaRef.current?.focus();
                }}
              >
                <Paperclip size={14} />
              </button>

              <div className="flex-1" />

              {running ? (
                <button
                  onClick={onAbort}
                  className="w-8 h-8 rounded-full bg-danger/80 flex items-center justify-center text-white hover:bg-danger transition-colors"
                  title="Stop"
                >
                  <Square size={14} fill="currentColor" />
                </button>
              ) : (
                <button
                  onClick={onSend}
                  disabled={disabled}
                  className="w-8 h-8 rounded-full bg-foreground flex items-center justify-center text-black disabled:opacity-20 hover:bg-foreground/90 transition-all"
                  title="Send (Enter)"
                >
                  <ArrowUp size={16} strokeWidth={2.5} />
                </button>
              )}
            </div>
          </DragDropZone>
        </div>

        {/* Selector Layer: project, model, approval mode */}
        <div className="mt-2 flex items-center gap-1">
          <ProjectSelector
            projects={projects}
            selectedId={selectedProjectId}
            fallbackLabel={projectFallbackLabel}
            onSelect={onProjectChange}
          />
          <ModelSelector value={selectedModel} onChange={onModelChange} />
          <ApprovalModeSelector value={approvalMode} onChange={onApprovalModeChange} />
        </div>

      </div>
      </div>
      {previewAttachment && (
        <ImageViewerModal
          src={`data:${previewAttachment.mimeType};base64,${previewAttachment.data}`}
          alt="Attachment preview"
          onClose={() => setPreviewAttachment(null)}
        />
      )}
    </>
  );
}
