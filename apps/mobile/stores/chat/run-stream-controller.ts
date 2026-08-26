import {
  startNativeChatStream,
  startNativeGetStream,
} from "@/utils/native-stream";
import { getRunStreamPath } from "@console/api";
import type { AgentSessionEvent, RunPromptDto } from "@console/types";
import { isAbortError } from "./chat-stream-runner";

/** Reconnect attempts before giving up on a dropped run stream. */
const MAX_RECONNECT_ATTEMPTS = 3;
/** Exponential backoff between reconnect attempts (1s, 2s, 4s). */
const RECONNECT_BACKOFF_MS = [1_000, 2_000, 4_000];

export interface RunStreamControllerDeps {
  setSessions: (
    updater: (
      sessions: Record<string, import("@/types").ChatSessionState>,
    ) => Record<string, import("@/types").ChatSessionState>,
  ) => void;
  handleEvent: (event: AgentSessionEvent) => void;
  /** Append an error bubble to the transcript. */
  markError: (message: string) => void;
  /** Finalize the local run state (called exactly once per run). */
  finalize: (hadError: boolean) => void;
  baseUrl: () => string;
}

type StreamOpener = () => () => void;

/**
 * Owns the lifecycle of one session's run stream: the initial POST /run SSE
 * pipe plus reconnect-with-resume attempts against the re-attach endpoint.
 *
 * Guarantees:
 *  - `deps.finalize` runs at most once per controller lifetime.
 *  - A transport failure triggers up to MAX_RECONNECT_ATTEMPTS resumptions
 *    (`GET /run/stream?since=<lastSeq>`); the server's 409 ("no active run")
 *    or a terminal `done`/`aborted` frame finalizes cleanly instead of
 *    surfacing an error.
 *  - `cancel()` (user pressed stop) kills pending reconnect timers and marks
 *    everything inert so late callbacks can't resurrect the run UI.
 */
export class RunStreamController {
  private closeActive?: () => void;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private finished = false;
  private userCancelled = false;
  private hadError = false;
  /** Set between a transport failure and the paired onEnd callback. */
  private awaitingEndAfterFailure = false;
  private attempts = 0;
  private lastSeq?: number;

  constructor(
    public readonly sessionId: string,
    private readonly deps: RunStreamControllerDeps,
  ) {}

  get isActive(): boolean {
    return !this.finished && !this.userCancelled;
  }

  /** Start a fresh agent run (POST /api/sessions/:id/run). */
  startRun(body: Partial<RunPromptDto> & Record<string, unknown>): void {
    const baseUrl = this.deps.baseUrl();
    this.open(() =>
      startNativeChatStream(
        `chat-${this.sessionId}-${Date.now()}`,
        `${baseUrl}/api/sessions/${this.sessionId}/run`,
        body,
        this.callbacks(),
      ),
    );
  }

  /**
   * Attach to the server-side active run. With `since`, buffered events newer
   * than that seq are replayed first; without it, go live immediately
   * (server replays from seq 0 of the current run's buffer when since=0).
   */
  attach(since?: number): void {
    const baseUrl = this.deps.baseUrl();
    this.open(() =>
      startNativeGetStream(
        `reattach-${this.sessionId}-${Date.now()}`,
        `${baseUrl}${getRunStreamPath(this.sessionId, since)}`,
        this.callbacks(),
      ),
    );
  }

  /** User-initiated stop: kill streams/timers and mark inert. */
  cancel(): void {
    this.userCancelled = true;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = undefined;
    this.closeActive?.();
    this.closeActive = undefined;
  }

  private open(opener: StreamOpener): void {
    // One live stream at a time — replacing closes the previous one.
    this.closeActive?.();
    this.closeActive = opener();
  }

  private callbacks() {
    return {
      onEvent: (event: AgentSessionEvent, meta?: { seq?: number }) => {
        if (meta?.seq != null) this.lastSeq = meta.seq;

        if (event.type === "done" || event.type === "aborted") {
          // Terminal frames arrive only on extendedFrames attach streams.
          this.finish(false);
          return;
        }
        if (event.type === "error" && !isAbortError(event.error.message)) {
          this.hadError = true;
        }
        this.deps.handleEvent(event);
      },

      onError: (message: string, info?: { statusCode?: number }) => {
        if (this.userCancelled || this.finished) return;

        // 409 = the run settled server-side while we were disconnected.
        // Not a failure — finalize gracefully without reconnect churn.
        if (info?.statusCode === 409) {
          this.finish(this.hadError);
          return;
        }

        if (this.attempts < MAX_RECONNECT_ATTEMPTS) {
          this.attempts += 1;
          this.awaitingEndAfterFailure = true;
          const delay = RECONNECT_BACKOFF_MS[this.attempts - 1] ?? 4_000;
          this.reconnectTimer = setTimeout(() => {
            this.reconnectTimer = undefined;
            if (this.userCancelled || this.finished) return;
            this.attach(this.lastSeq);
          }, delay);
          return;
        }

        // Out of retries (or run gone): surface the error; the paired onEnd
        // finalizes with hadError=true.
        this.hadError = true;
        this.deps.markError(message);
      },

      onEnd: (abortedFlag: boolean) => {
        if (this.userCancelled || this.finished) return;
        if (this.awaitingEndAfterFailure) {
          // Native wrapper reports failure via onError then onEnd(false);
          // a reconnect is already scheduled — don't finalize here.
          this.awaitingEndAfterFailure = false;
          return;
        }
        // Normal stream end: for POST /run pipes the server closes after the
        // run completes; attach streams close once the run settles. Either
        // way, end-of-stream means the run is done locally.
        void abortedFlag;
        this.finish(this.hadError);
      },
    };
  }

  private finish(hadError: boolean): void {
    if (this.finished) return;
    this.finished = true;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = undefined;
    this.closeActive?.();
    this.closeActive = undefined;
    this.deps.finalize(hadError);
  }
}

// --- Per-session registry ---

const controllers = new Map<string, RunStreamController>();

export function getOrCreateController(
  sessionId: string,
  deps: RunStreamControllerDeps,
): RunStreamController {
  let controller = controllers.get(sessionId);
  if (!controller) {
    controller = new RunStreamController(sessionId, deps);
    controllers.set(sessionId, controller);
  }
  return controller;
}

export function getController(sessionId: string): RunStreamController | undefined {
  return controllers.get(sessionId);
}

export function removeController(sessionId: string): void {
  controllers.delete(sessionId);
}
