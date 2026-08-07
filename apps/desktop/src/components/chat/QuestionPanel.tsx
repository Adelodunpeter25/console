import React from "react";
import { HelpCircle } from "lucide-react";
import type { AskQuestionRequest } from "@console/types";

interface QuestionPanelProps {
  request: AskQuestionRequest;
  /** Total questions in the batch (for the "Question X of Y" indicator). */
  total: number;
  /** 1-based index of the current question in the batch. */
  index: number;
  /** True when this is the last question — the primary button becomes "Submit all". */
  isLast: boolean;
  submitting: boolean;
  /** Called with the user's answer to this question (selected options or typed text). */
  onAnswer: (answer: string | string[]) => void;
  /** Called when the user chooses to skip this question. */
  onSkip: () => void;
}

/**
 * Renders a single ask-tool question: optional multiple-choice options, a
 * free-text input that is ALWAYS available, and a Skip button when the
 * question is skippable. The primary button advances the wizard (or submits
 * everything on the last question).
 */
export function QuestionPanel({
  request,
  total,
  index,
  isLast,
  submitting,
  onAnswer,
  onSkip,
}: QuestionPanelProps) {
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  const [customAnswer, setCustomAnswer] = React.useState("");

  const hasOptions = request.options != null && request.options.length > 0;
  const hasAnswer = customAnswer.trim().length > 0 || selected.size > 0;

  const toggle = (option: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (request.isMultiSelect) {
        if (next.has(option)) next.delete(option);
        else next.add(option);
      } else {
        next.clear();
        next.add(option);
      }
      return next;
    });
  };

  const getAnswer = (): string | string[] | null => {
    if (customAnswer.trim()) return customAnswer.trim();
    if (selected.size === 0) return null;
    return request.isMultiSelect ? [...selected] : [...selected][0]!;
  };

  const handlePrimary = () => {
    const answer = getAnswer();
    if (answer !== null) onAnswer(answer);
  };

  return (
    <div className="rounded-xl border border-primary/20 bg-card px-4 py-3 space-y-3">
      <div className="flex items-center gap-2.5">
        <HelpCircle size={16} className="text-primary shrink-0" />
        <span className="text-sm font-medium text-foreground">{request.question}</span>
        {total > 1 && (
          <span className="text-[11px] text-foreground-muted shrink-0">
            {index} of {total}
          </span>
        )}
      </div>

      {hasOptions && (
        <div className="space-y-1.5 pl-6">
          {request.options!.map((option) => {
            const isSelected = selected.has(option);
            return (
              <button
                key={option}
                onClick={() => toggle(option)}
                className={`w-full text-left flex items-center gap-2.5 px-3 py-2 rounded-md border text-sm transition-colors ${
                  isSelected
                    ? "border-primary/40 bg-primary/10 text-foreground"
                    : "border-border bg-transparent text-foreground-secondary hover:bg-white/5"
                }`}
              >
                <span
                  className={`shrink-0 w-4 h-4 ${
                    request.isMultiSelect ? "rounded-sm" : "rounded-full"
                  } border-2 flex items-center justify-center ${
                    isSelected ? "border-primary bg-primary/20" : "border-foreground-muted"
                  }`}
                >
                  {isSelected && <span className="w-1.5 h-1.5 rounded-full bg-primary" />}
                </span>
                {option}
              </button>
            );
          })}
        </div>
      )}

      <div className="pl-6">
        <input
          type="text"
          value={customAnswer}
          onChange={(e) => setCustomAnswer(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && hasAnswer) handlePrimary();
          }}
          placeholder={hasOptions ? "Or type your own answer…" : "Type your answer…"}
          className="w-full px-3 py-2 rounded-md border border-border bg-transparent text-sm text-foreground placeholder:text-foreground-muted focus:outline-none focus:border-primary/50"
        />
      </div>

      <div className="flex items-center gap-2 pl-6">
        <button
          onClick={handlePrimary}
          disabled={!hasAnswer || submitting}
          className="px-4 py-1.5 rounded-md bg-primary/10 hover:bg-primary/20 border border-primary/30 text-primary text-xs font-medium transition-colors disabled:opacity-40"
        >
          {submitting ? "Sending…" : isLast ? (total > 1 ? "Submit all" : "Submit") : "Next"}
        </button>
        {request.skippable !== false && (
          <button
            onClick={onSkip}
            disabled={submitting}
            className="px-4 py-1.5 rounded-md bg-transparent hover:bg-white/5 border border-border text-foreground-muted hover:text-foreground text-xs font-medium transition-colors disabled:opacity-40"
          >
            Skip
          </button>
        )}
      </div>
    </div>
  );
}
