import React from "react";
import { HelpCircle, ShieldCheck, ShieldX, Loader2 } from "lucide-react";
import type { AskQuestionRequest, PermissionRequest } from "@console/types";
import { useChatStore } from "../../store/useChatStore";

/* ------------------------------------------------------------------ */
/* Permission request panel                                            */
/* ------------------------------------------------------------------ */

interface PermissionPanelProps {
  request: PermissionRequest;
  sessionId: string;
}

function PermissionPanel({ request, sessionId }: PermissionPanelProps) {
  const approvePermission = useChatStore((s) => s.approvePermission);
  const [submitting, setSubmitting] = React.useState(false);

  const handleApprove = async (allow: boolean) => {
    setSubmitting(true);
    await approvePermission(sessionId, request.requestId, allow);
    // The store removes this request after the server confirms the decision.
  };

  return (
    <div className="rounded-xl border border-warning/30 bg-warning-muted px-4 py-3 space-y-3">
      <div className="flex items-center gap-2.5">
        <ShieldCheck size={16} className="text-warning shrink-0" />
        <span className="text-sm font-medium text-foreground">
          {request.requiresUpgrade ? "Upgrade permission required: " : "Permission required: "}
          <span className="font-mono text-warning">{request.toolName}</span>
        </span>
      </div>
      {request.reason && <p className="text-xs text-foreground-secondary pl-6">{request.reason}</p>}
      {request.args != null && (
        <pre className="text-xs font-mono text-foreground-muted whitespace-pre-wrap break-all bg-black/30 rounded p-2 max-h-40 overflow-y-auto ml-6">
          {JSON.stringify(request.args, null, 2)}
        </pre>
      )}
      <div className="flex items-center gap-2 pl-6">
        <button
          onClick={() => handleApprove(true)}
          disabled={submitting}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-success/15 hover:bg-success/25 border border-success/30 text-success text-xs font-medium transition-colors disabled:opacity-50"
        >
          {submitting ? <Loader2 size={13} className="animate-spin" /> : <ShieldCheck size={13} />}
          {request.requiresUpgrade ? "Allow once" : "Allow"}
        </button>
        <button
          onClick={() => handleApprove(false)}
          disabled={submitting}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-danger/15 hover:bg-danger/25 border border-danger/30 text-danger text-xs font-medium transition-colors disabled:opacity-50"
        >
          {submitting ? <Loader2 size={13} className="animate-spin" /> : <ShieldX size={13} />}
          Deny
        </button>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Question panel                                                      */
/* ------------------------------------------------------------------ */

interface QuestionPanelProps {
  request: AskQuestionRequest;
  sessionId: string;
}

function QuestionPanel({ request, sessionId }: QuestionPanelProps) {
  const answerQuestion = useChatStore((s) => s.answerQuestion);
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  const [customAnswer, setCustomAnswer] = React.useState("");
  const [submitting, setSubmitting] = React.useState(false);

  const hasOptions = request.options != null && request.options.length > 0;

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

  const submitAnswer = async (answer: string | string[]) => {
    setSubmitting(true);
    await answerQuestion(sessionId, request.requestId, answer);
  };

  const handleSubmit = async () => {
    if (customAnswer.trim()) {
      await submitAnswer(customAnswer.trim());
      return;
    }
    if (selected.size === 0) return;
    await submitAnswer(request.isMultiSelect ? [...selected] : [...selected][0]!);
  };

  const handleSkip = async () => {
    await submitAnswer("");
  };

  const canSubmit = customAnswer.trim().length > 0 || selected.size > 0;

  return (
    <div className="rounded-xl border border-primary/20 bg-card px-4 py-3 space-y-3">
      <div className="flex items-center gap-2.5">
        <HelpCircle size={16} className="text-primary shrink-0" />
        <span className="text-sm font-medium text-foreground">{request.question}</span>
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
            if (e.key === "Enter" && canSubmit) handleSubmit();
          }}
          placeholder={hasOptions ? "Or type your own answer…" : "Type your answer…"}
          className="w-full px-3 py-2 rounded-md border border-border bg-transparent text-sm text-foreground placeholder:text-foreground-muted focus:outline-none focus:border-primary/50"
        />
      </div>

      <div className="flex items-center gap-2 pl-6">
        <button
          onClick={handleSubmit}
          disabled={!canSubmit || submitting}
          className="px-4 py-1.5 rounded-md bg-primary/10 hover:bg-primary/20 border border-primary/30 text-primary text-xs font-medium transition-colors disabled:opacity-40"
        >
          {submitting ? "Sending…" : "Submit"}
        </button>
        {request.skippable && (
          <button
            onClick={handleSkip}
            disabled={submitting}
            className="px-4 py-1.5 rounded-md bg-transparent hover:bg-white/5 border border-border text-foreground-muted text-xs font-medium transition-colors disabled:opacity-40"
          >
            Skip
          </button>
        )}
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Dispatcher — renders whichever interaction is pending               */
/* ------------------------------------------------------------------ */

interface InteractionPanelProps {
  sessionId: string;
}

const EMPTY_PERMISSIONS: any[] = [];

/**
 * Renders the pending agent interaction (question or permission request)
 * above the composer. Returns null when nothing is pending.
 */
export function InteractionPanel({ sessionId }: InteractionPanelProps) {
  const pendingQuestion = useChatStore((s) => s.sessions[sessionId]?.pendingQuestion ?? null);
  const pendingPermissions = useChatStore((s) => s.sessions[sessionId]?.pendingPermissions ?? EMPTY_PERMISSIONS);

  React.useEffect(() => {
    const request = pendingPermissions[0]?.request;
    if (request) {
      console.info("[permission] panel rendering request", {
        sessionId,
        requestId: request.requestId,
        toolName: request.toolName,
        tier: request.tier,
        requiresUpgrade: request.requiresUpgrade,
      });
    }
  }, [pendingPermissions, sessionId]);

  if (pendingPermissions.length > 0) {
    return (
      <PermissionPanel
        key={pendingPermissions[0].request.requestId}
        request={pendingPermissions[0].request}
        sessionId={sessionId}
      />
    );
  }
  if (pendingQuestion) {
    return <QuestionPanel request={pendingQuestion.request} sessionId={sessionId} />;
  }
  return null;
}
