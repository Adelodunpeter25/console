// One chat tab's content: the transcript plus composer, backed by the real
// session. Ported from the desktop's per-pane transcript wiring — the pane
// loads the session's message tail, streams runs through the native bridge
// (`Console.runPrompt`), and reconciles with the canonical history when a
// run settles (desktop `run.rs`'s `wait_until_settled` step).
//
// Components run once in nsh, so the controller below is created once per
// mount and everything dynamic reads its signals inside JSX.

import { signal, Text, View } from "nsh";
import { Console } from "console-nsh/native";
import { Composer } from "../common/composer";
import { TranscriptView, createTranscriptState } from "./transcript_view";
import { applyRunEvent, type ChatRunEvent } from "./run_events";
import type { AgentMessage } from "./transcript_model";

/// One chat tab's rendered surface: the transcript, the trailing error line,
/// and the composer. The controller (created once per mount) owns the state.
export function SessionPane(props: { controller: ChatController }) {
  const controller = props.controller;
  const draft = signal("");
  return (
    <View class="flex-1 flex-col">
      {controller.error() != null ? (
        <View class="flex-none px-3 py-1">
          <Text class="text-[11px] text-red-500">{controller.error()}</Text>
        </View>
      ) : null}
      <TranscriptView state={controller.state} />
      <View class="flex-none border-t border-border">
        <Composer
          value={draft()}
          onChange={(text: string) => draft.set(text)}
          onSend={() => {
            const text = draft();
            draft.set("");
            void controller.send(text);
          }}
          runState={controller.state.isStreaming() ? "running" : "ready"}
        />
      </View>
    </View>
  );
}

export interface ChatController {
  state: ReturnType<typeof createTranscriptState>;
  /// Sends a prompt as a new run. Optimistically appends the user message,
  /// streams the run's events into the transcript, then reconciles with the
  /// server's canonical history once the run settles.
  send(prompt: string): Promise<void>;
  /// Reloads the tail page (the canonical history after a settle or an
  /// external change).
  refresh(): Promise<void>;
  error: () => string | null;
}

interface SessionDetailJson {
  messages: AgentMessage[];
  hasMore?: boolean;
  nextCursor?: number | null;
}

export function createChatController(baseUrl: string, sessionId: string): ChatController {
  const state = createTranscriptState();
  const loadError = signal<string | null>(null);

  async function load(): Promise<void> {
    try {
      const detail = JSON.parse(await Console.chatMessages(baseUrl, sessionId)) as SessionDetailJson;
      state.setMessages(detail.messages ?? []);
      state.setPagination(detail.hasMore ?? false, detail.nextCursor ?? null);
      loadError.set(null);
    } catch (err) {
      loadError.set(err instanceof Error ? err.message : String(err));
    }
  }

  async function send(prompt: string): Promise<void> {
    const trimmed = prompt.trim();
    if (trimmed === "") return;
    // Optimistic user row; canonical ids land with the settle refresh.
    state.pushMessage({
      type: "user",
      content: trimmed,
      createdAt: Math.floor(Date.now() / 1000),
    });
    state.beginStreaming();
    try {
      await Console.runPrompt(baseUrl, sessionId, { prompt: trimmed }, (event: unknown) => {
        applyRunEvent(state, event as ChatRunEvent);
      });
      // The run's stream ended: swap the streamed shadow for the server's
      // canonical messages (desktop `run.rs`'s settle step). A shorter
      // canonical list than what we streamed means the server has not
      // flushed yet — adopt ids instead of clobbering live rows.
      const detail = JSON.parse(await Console.chatMessages(baseUrl, sessionId)) as SessionDetailJson;
      if (detail.messages.length >= state.messages().length) {
        state.setMessages(detail.messages);
      } else {
        state.adoptUserMessageIds(detail.messages);
      }
      state.setPagination(detail.hasMore ?? false, detail.nextCursor ?? null);
      loadError.set(null);
    } catch (err) {
      state.finishStreaming();
      loadError.set(err instanceof Error ? err.message : String(err));
    }
  }

  void load();

  return { state, send, refresh: load, error: loadError };
}
