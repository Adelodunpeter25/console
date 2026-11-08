// The floating jump-to-prompt pill, split out of `transcript_view.tsx`: the
// always-visible controls pill and its popover of past prompts, each row
// labeled with its age. The popover itself opens on the trigger press (nsh's
// `Popover` owns visibility); the press also refetches the prompt list when
// stale, like the desktop's toggle handler did.

import { List, signal, Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Popover } from "nsh/base";
import { Icon } from "../common/icons";
import { formatTimeAgo } from "../utils/time";
import type { TranscriptState } from "./transcript_state";

const PROMPT_ROW_HEIGHT = 34;
const PROMPT_MAX_ROWS = 7;

/// The popup's content, its own component so every branch reads its signals
/// inside JSX (the popover mounts its content once). The list mounts only
/// once prompts exist, so `initialScrollIndex` lands on the newest.
function PromptPopupBody(props: {
  state: TranscriptState;
  loading: () => boolean;
  onJump: (fromEnd: number) => void;
}) {
  const state = props.state;
  return (
    <View class="flex-col">
      {state.prompts().length === 0 ? (
        <View class="px-3 py-[7px]">
          <Text class="text-[13px] text-muted-foreground">
            {props.loading() ? "Loading…" : "No prompts"}
          </Text>
        </View>
      ) : (
        <View
          style={{
            height:
              Math.min(state.prompts().length, PROMPT_MAX_ROWS) * PROMPT_ROW_HEIGHT,
          }}
        >
          <List
            class="h-full w-full"
            each={state.prompts()}
            keyExtractor={(prompt) => prompt.id}
            initialScrollIndex={state.prompts().length - 1}
          >
            {(item, index) => {
              const prompt = item();
              const fromEnd = state.prompts().length - 1 - index();
              const age = formatTimeAgo(prompt.createdAt);
              return (
                <View
                  class="h-[34px] min-h-[34px] w-full flex-none flex-row items-center gap-3 overflow-hidden rounded-lg px-3 hover:bg-accent"
                  role="button"
                  accessibilityLabel={`Jump to prompt ${prompt.id}`}
                  onPress={() => props.onJump(fromEnd)}
                >
                  <View class="min-w-0 flex-1 overflow-hidden">
                    <Text class="truncate text-[13px] text-foreground">
                      {prompt.preview.replace(/\s+/g, " ").trim()}
                    </Text>
                  </View>
                  {prompt.createdAt > 0 ? (
                    <Text class="flex-none text-[12px] text-muted-foreground">
                      {age.includes(" ") ? age : `${age} ago`}
                    </Text>
                  ) : null}
                </View>
              );
            }}
          </List>
        </View>
      )}
    </View>
  );
}

export function PromptJump(props: {
  state: TranscriptState;
  /// Fetches the chat's full prompt list when the popup opens stale.
  onRequestPrompts?: () => Promise<void> | void;
  /// Jumps to a prompt, given as its position counted from the newest
  /// (0 = newest).
  onJumpToPrompt?: (fromEnd: number) => void;
  /// Whether the list is scrolled away from its end (shows the arrow).
  showScrollToBottom?: () => boolean;
  onScrollToBottom?: () => void;
}) {
  const state = props.state;

  const loading = signal(false);
  const requestPrompts = () => {
    if (!(state.promptsStale() || state.prompts().length === 0)) return;
    state.promptsStale.set(false);
    loading.set(true);
    void (async () => {
      try {
        await props.onRequestPrompts?.();
      } finally {
        loading.set(false);
      }
    })();
  };

  const hasPrompts = () => state.messages().length > 0;
  // The pill shows when the chat has messages; the guard reads inside JSX
  // so the pill appears when the first message arrives (the component runs
  // once).
  return (
    <View class="pointer-events-none absolute inset-0">
      {hasPrompts() ? (
        <View
          class={
            "pointer-events-auto absolute bottom-3 right-4 flex-row items-center overflow-hidden rounded-[17px] border border-border bg-popover " +
            (state.promptsOpen() ? "" : "opacity-0 group-hover:opacity-100")
          }
        >
      <Popover
        placement="top"
        align="end"
        offset={6}
        popupClass="rounded-[10px] border border-border bg-popover p-1 shadow-md"
        popupStyle={{ width: 340 }}
        open={state.promptsOpen()}
        onOpenChange={(e) => {
          state.promptsOpen.set(e.open);
          if (e.open) requestPrompts();
        }}
        trigger={
          <View
            class="h-8 w-8 flex-row items-center justify-center hover:bg-accent"
            role="button"
            accessibilityLabel="Jump to prompt"
          >
            <Icon name="List" size={12} color={token("muted-foreground")} />
          </View>
        }
      >
        <PromptPopupBody
          state={state}
          loading={loading}
          onJump={(fromEnd) => {
            state.promptsOpen.set(false);
            props.onJumpToPrompt?.(fromEnd);
          }}
        />
        </Popover>
        {props.showScrollToBottom?.() ? (
          <View class="flex-row items-center">
            <View class="h-4 w-px flex-none bg-border" />
            <View
              class="h-8 w-8 flex-row items-center justify-center hover:bg-accent"
              role="button"
              accessibilityLabel="Scroll to bottom"
              onPress={props.onScrollToBottom}
            >
              <Icon name="ArrowDown" size={12} color={token("muted-foreground")} />
            </View>
          </View>
        ) : null}
        </View>
      ) : null}
    </View>
  );
}
