import React from "react";
import { ArrowUp, Square, Paperclip } from "lucide-react";
import type { ApprovalMode, ProjectInfo } from "@console/types";
import { ModelSelector, ApprovalModeSelector, ProjectSelector } from "../common";
import { ComposerAutocomplete } from "./ComposerAutocomplete";

interface ComposerProps {
  value: string;
  onChange: (value: string) => void;
  onSend: () => void;
  onAbort: () => void;
  running: boolean;
  disabled: boolean;
  selectedModel: string | null;
  onModelChange: (modelId: string) => void;
  approvalMode: ApprovalMode;
  onApprovalModeChange: (mode: ApprovalMode) => void;
  projects: ProjectInfo[];
  selectedProjectId: string | null;
  projectFallbackLabel: string;
  onProjectChange: (project: ProjectInfo) => void;
  /** Active session id — scopes slash-command + @-file autocomplete. */
  sessionId?: string | null;
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
  onModelChange,
  approvalMode,
  onApprovalModeChange,
  projects,
  selectedProjectId,
  projectFallbackLabel,
  onProjectChange,
  sessionId,
}: ComposerProps) {
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);

  // Auto-grow textarea height up to a max.
  React.useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
      textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 160)}px`;
    }
  }, [value]);

  return (
    <div className="px-6 pb-4 pt-2">
      <div className="max-w-3xl mx-auto">
        <div className="relative">
          <ComposerAutocomplete
            value={value}
            sessionId={sessionId ?? null}
            onPick={onChange}
            textareaRef={textareaRef}
          />
          <div className="bg-card border border-border rounded-2xl focus-within:border-border-strong transition-colors">
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
                className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-foreground-muted hover:text-foreground-secondary hover:bg-white/5 transition-colors"
                title="Attach file — type @ to search"
                onClick={() => textareaRef.current?.focus()}
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
          </div>
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
  );
}
