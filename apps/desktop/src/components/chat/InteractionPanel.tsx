import React from "react";
import { ShieldCheck, ShieldX, Loader2 } from "lucide-react";
import type { AskQuestionRequest, PermissionRequest } from "@console/types";
import { useChatStore } from "../../store/useChatStore";
import { QuestionPanel } from "./QuestionPanel";

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
/* Question wizard                                                     */
/* ------------------------------------------------------------------ */

/**
 * Per-question wizard over the pending question queue. Renders one question
 * at a time; "Next" collects the answer locally and advances; the last
 * question's button becomes "Submit all", which sends every collected answer.
 */
function QuestionWizard({ questions, sessionId }: { questions: AskQuestionRequest[]; sessionId: string }) {
  const answerQuestion = useChatStore((s) => s.answerQuestion);
  const [index, setIndex] = React.useState(0);
  const [submitting, setSubmitting] = React.useState(false);
  // Always-current collected answers (ref avoids stale state in the submit-all
  // handler, which fires in the same tick as the last answer being recorded).
  const answersRef = React.useRef<Map<string, string | string[]>>(new Map());

  const current = questions[index]!;
  const isLast = index === questions.length - 1;

  const recordAnswer = (requestId: string, answer: string | string[]) => {
    answersRef.current.set(requestId, answer);
  };

  const handleNext = () => {
    setIndex((i) => Math.min(i + 1, questions.length - 1));
  };

  const handleSkip = () => {
    recordAnswer(current.requestId, "");
    setIndex((i) => Math.min(i + 1, questions.length - 1));
  };

  const handleSubmitAll = async () => {
    setSubmitting(true);
    // Send every collected answer through the existing per-question endpoint.
    for (const [requestId, answer] of answersRef.current.entries()) {
      await answerQuestion(sessionId, requestId, answer);
    }
    setSubmitting(false);
  };

  if (!current) return null;

  return (
    <QuestionPanel
      key={current.requestId}
      request={current}
      total={questions.length}
      index={index + 1}
      isLast={isLast}
      submitting={submitting}
      onAnswer={(answer) => {
        recordAnswer(current.requestId, answer);
        if (isLast) handleSubmitAll();
        else handleNext();
      }}
      onSkip={handleSkip}
    />
  );
}

/* ------------------------------------------------------------------ */
/* Dispatcher — renders whichever interaction is pending               */
/* ------------------------------------------------------------------ */

interface InteractionPanelProps {
  sessionId: string;
}

const EMPTY_PERMISSIONS: any[] = [];
const EMPTY: any[] = [];

/**
 * Renders the pending agent interaction (question or permission request)
 * above the composer. Returns null when nothing is pending.
 */
export function InteractionPanel({ sessionId }: InteractionPanelProps) {
  const pendingQuestions = useChatStore((s) => s.sessions[sessionId]?.pendingQuestions ?? EMPTY);
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
  if (pendingQuestions.length > 0) {
    return (
      <QuestionWizard
        key={pendingQuestions[0].request.batchId ?? pendingQuestions[0].request.requestId}
        questions={pendingQuestions.map((q) => q.request)}
        sessionId={sessionId}
      />
    );
  }
  return null;
}
