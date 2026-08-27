import type { AskQuestionRequest, PermissionRequest } from "@console/types";
import type { RunEventHub } from "@console/utils";

export class DecisionManager {
  private static readonly DECISION_TIMEOUT_MS = 10 * 60 * 1000;

  private pendingQuestions = new Map<
    string,
    {
      sessionId: string;
      resolve: (answer: string | string[]) => void;
      reject: (err: unknown) => void;
    }
  >();

  private pendingApprovals = new Map<
    string,
    {
      sessionId: string;
      resolve: (allow: boolean) => void;
      reject: (err: unknown) => void;
    }
  >();

  createAskHandler(sessionId: string, hub: RunEventHub) {
    return (request: AskQuestionRequest): Promise<string | string[]> => {
      return new Promise<string | string[]>((resolve, reject) => {
        this.pendingQuestions.set(request.requestId, { sessionId, resolve, reject });
        hub.broadcast({ type: "askQuestion", request });
        this.startDecisionTimeout(request.requestId, sessionId, "question");
      });
    };
  }

  createApprovalHandler(sessionId: string) {
    return (req: PermissionRequest): Promise<boolean> => {
      return new Promise<boolean>((resolve, reject) => {
        this.pendingApprovals.set(req.requestId, { sessionId, resolve, reject });
        this.startDecisionTimeout(req.requestId, sessionId, "permission");
      });
    };
  }

  answerQuestion(sessionId: string, requestId: string, answer: string | string[]): boolean {
    const pending = this.pendingQuestions.get(requestId);
    if (!pending || pending.sessionId !== sessionId) return false;
    this.pendingQuestions.delete(requestId);
    pending.resolve(answer);
    return true;
  }

  approvePermission(sessionId: string, requestId: string, allow: boolean): boolean {
    const pending = this.pendingApprovals.get(requestId);
    if (!pending || pending.sessionId !== sessionId) return false;
    this.pendingApprovals.delete(requestId);
    pending.resolve(allow);
    return true;
  }

  rejectAllForSession(sessionId: string, reason: string): void {
    for (const [requestId, pending] of this.pendingQuestions) {
      if (pending.sessionId !== sessionId) continue;
      this.pendingQuestions.delete(requestId);
      pending.reject(new Error(reason));
    }
    for (const [requestId, pending] of this.pendingApprovals) {
      if (pending.sessionId !== sessionId) continue;
      this.pendingApprovals.delete(requestId);
      pending.reject(new Error(reason));
    }
  }

  private startDecisionTimeout(
    requestId: string,
    sessionId: string,
    kind: "question" | "permission",
  ): void {
    const timer = setTimeout(() => {
      const pending =
        kind === "question"
          ? this.pendingQuestions.get(requestId)
          : this.pendingApprovals.get(requestId);
      if (!pending || pending.sessionId !== sessionId) return;
      if (kind === "question") {
        this.pendingQuestions.delete(requestId);
      } else {
        this.pendingApprovals.delete(requestId);
      }
      pending.reject(
        new Error(
          `${kind === "question" ? "Question" : "Permission request"} timed out waiting for a decision.`,
        ),
      );
    }, DecisionManager.DECISION_TIMEOUT_MS);
    if (typeof timer === "object") timer.unref?.();
  }
}
