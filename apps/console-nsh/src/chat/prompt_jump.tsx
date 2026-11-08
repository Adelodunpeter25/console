// The floating jump-to-prompt pill, split out of `transcript_view.tsx`: the
// always-visible controls pill and its popover of past prompts, each row
// labeled with its age. The popover itself opens on the trigger press (nsh's
// `Popover` owns visibility); the press also refetches the prompt list when
// stale, like the desktop's toggle handler did.

import { signal, Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Popover } from "nsh/base";
import { Icon } from "../common/icons";
import { formatTimeAgo } from "../utils/time";
import type { TranscriptState } from "./transcript_state";

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

  const promptRows = (): JSX.Element[] => {
    const list = state.prompts();
    return list
      .map((prompt, at) => ({ prompt, fromEnd: list.length - 1 - at }))
      .map(({ prompt, fromEnd }) => (
        <View
          class="h-[34px] w-full flex-none flex-row items-center gap-3 rounded-lg px-3 hover:bg-accent"
          role="button"
          accessibilityLabel={`Jump to prompt ${prompt.id}`}
          onPress={() => {
            state.promptsOpen.set(false);
            props.onJumpToPrompt?.(fromEnd);
          }}
        >
          <View class="min-w-0 flex-1 overflow-hidden">
            <Text class="text-[13px] text-foreground">{prompt.preview}</Text>
          </View>
          {(() => {
            // "just now" and dates already read fine; only "3h" needs "ago".
            const age = formatTimeAgo(prompt.createdAt);
            return prompt.createdAt > 0 ? (
              <Text class="flex-none text-[12px] text-muted-foreground">
                {age.includes(" ") ? age : `${age} ago`}
              </Text>
            ) : null;
          })()}
        </View>
      ));
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
        popupStyle={{ width: 300, maxHeight: 7 * 34 }}
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
        {loading() && state.prompts().length === 0 ? (
          <View class="px-3 py-[7px]">
            <Text class="text-[13px] text-muted-foreground">Loading…</Text>
          </View>
        ) : (
          <View class="flex-col">{promptRows()}</View>
        )}
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
