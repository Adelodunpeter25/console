// The chat composer, ported from console-ui's `common/composer_view.rs`
// (first cut). Full-card variant: staged chips row, attachment thumbnails,
// a multiline input, and the footer of model / approval / thinking chips
// with attach and send. The popovers behind the model and approval chips
// (the dropdowns) and the floating compact variant land with the
// primitives pass; until then the chips press but don't open menus.
//
// NSH renders components once and tracks prop reads in JSX, so all values
// derived from props are read lazily (`()` thunks): normalizing a prop into
// a body-level const would freeze it at mount.

import { Image, Text, View, colorScheme, effect, token } from "nsh";
import { Input, Popover } from "nsh/base";
import type { InputRef } from "nsh/types";
import { Icon, type IconName } from "./icons";
import {
  ApprovalModeDropdown,
  approvalModeIcon,
  approvalModeLabel,
} from "./approval_selector";
import {
  ModelDropdownMenu,
  providerIconName,
  type CatalogProvider,
  type PickerTab,
  type SelectedModel,
  formatModelName,
} from "./model_picker";
export type {
  CatalogModel,
  CatalogProvider,
  PickerTab,
  SelectedModel,
} from "./model_picker";
import { keymap, signal } from "nsh";

/// Focus requests for the docked composer's input: bumped by the global
/// shortcut (cmd+l, the desktop's `FocusComposer`); every mounted Composer
/// watches it and takes focus. A signal, not a direct call, so the app
/// doesn't need a ref into whichever composer is on screen.
export const composerFocusRequest = signal(0);

export function focusComposer(): void {
  composerFocusRequest.set(composerFocusRequest() + 1);
}
import { ComposerAutocompleteRow } from "./composer_autocomplete_row";
import {
  detectTrigger,
  filterItems,
  itemInsertText,
  type SlashCommand,
  type SuggestionChat,
  type SuggestionFile,
  type AutocompleteItem,
} from "./composer_autocomplete";

/// A staged file comment chip above the composer's text field
/// (`composer_view.rs` `ComposerComment`).
export type ComposerComment = {
  id: number;
  path: string;
  label: string;
  onEdit?: () => void;
  onRemove?: () => void;
};

/// A staged browser element annotation chip (`ComposerAnnotation`).
export type ComposerAnnotation = {
  id: string;
  label: string;
  tooltip?: string;
  onRemove?: () => void;
};

/// A staged image attachment (`ImageAttachment` of console-core; the app
/// shows it by URL until the attachment service moves over).
export type ComposerAttachment = {
  index: number;
  url: string;
};

/// The run state the footer reflects (`ComposerRunState`): whether a chat
/// run is live (send becomes stop) or the composer is ready.
export type ComposerRunState = "ready" | "running";

// The approval modes and their presentation moved to `approval_selector.tsx`
// (console-ui's `approval_selector.rs`); re-exported here for the call sites
// that import them from the composer.
export {
  APPROVAL_MODES,
  approvalModeDescription,
  approvalModeFromValue,
  approvalModeIcon,
  approvalModeLabel,
  approvalModeValue,
} from "./approval_selector";
import type { ApprovalMode } from "./approval_selector";
export type { ApprovalMode } from "./approval_selector";

/// The thinking levels of the desktop (`console-core`'s `ThinkingLevel`).
export const THINKING_LEVELS = [
  "None",
  "Minimal",
  "Low",
  "Medium",
  "High",
  "XHigh",
  "Max",
] as const;
export type ThinkingLevel = (typeof THINKING_LEVELS)[number];

export function thinkingLevelLabel(level: ThinkingLevel): string {
  return level === "None" ? "No thinking" : `Thinking: ${level.toLowerCase()}`;
}

/// Keyboard bindings owned by the open autocomplete popup: Up / Down move
/// the highlight, Enter / Tab confirm, Escape dismisses. A binding made
/// while a component renders is removed with the component, so these live
/// only while the popup shows — the platform consumes them before the
/// input's Return-to-submit (the desktop's `AUTOCOMPLETE_CONTEXT` bindings).
function AutocompleteKeys(props: {
  onMove: (by: number) => void;
  onConfirm: () => void;
  onDismiss: () => void;
}) {
  keymap.bind({
    up: () => props.onMove(-1),
    down: () => props.onMove(1),
    enter: () => props.onConfirm(),
    tab: () => props.onConfirm(),
    escape: () => props.onDismiss(),
  });
  return null;
}

/// History-recall bindings, mounted only while the input is empty and a
/// history exists: Up recalls the previous prompt, Down the next (the
/// desktop's composer does the same with an empty field).
function HistoryKeys(props: {
  onRecall: (by: number) => void;
}) {
  keymap.bind({
    up: () => props.onRecall(-1),
    down: () => props.onRecall(1),
  });
  return null;
}

export function Composer(props: {
  value: string;
  onChange: (text: string) => void;
  onSend: () => void;
  runState?: ComposerRunState;
  approvalMode?: ApprovalMode;
  onApprovalMode?: (mode: ApprovalMode) => void;
  /// The provider catalog for the model picker. Without it the model chip
  /// renders as a plain label (no popover), like the desktop's footer
  /// without a menu.
  providers?: CatalogProvider[];
  selectedModel?: SelectedModel | null;
  favorites?: Set<string>;
  onModelSelect?: (provider: string, modelId: string) => void;
  onFavorite?: (provider: string, modelId: string) => void;
  thinkingLevel?: ThinkingLevel;
  onCycleThinking?: () => void;
  attachments?: ComposerAttachment[];
  onRemoveAttachment?: (index: number) => void;
  comments?: ComposerComment[];
  annotations?: ComposerAnnotation[];
  /// Slash commands, workspace files and chat titles the autocomplete
  /// filters (the desktop's backend fetches stay with the host).
  commands?: SlashCommand[];
  files?: SuggestionFile[];
  chats?: SuggestionChat[];
  /// The `@` query being typed, for the host's per-query file search.
  onFileQuery?: (query: string) => void;
  /// Past prompts, oldest first. With an empty input, Up recalls them.
  promptHistory?: string[];
  placeholder?: string;
  testID?: string;
}) {
  const runState = () => props.runState ?? "ready";
  const isRunning = () => runState() === "running";
  const attachments = () => props.attachments ?? [];
  const comments = () => props.comments ?? [];
  const annotations = () => props.annotations ?? [];
  const commands = () => props.commands ?? [];
  const files = () => props.files ?? [];
  const chats = () => props.chats ?? [];
  const history = () => props.promptHistory ?? [];

  // The composer caret, tracked so autocomplete triggers detect against it
  // (the desktop reads the input entity's caret).
  const selection = signal({ start: 0, end: 0 });
  // The mounted input, for the cmd+l focus request.
  let inputRef: InputRef | null = null;
  effect(() => {
    if (composerFocusRequest() > 0) inputRef?.focus();
  });
  effect(() => {
    const trigger = detectTrigger(props.value, selection().start);
    if (trigger?.kind === "file") props.onFileQuery?.(trigger.query);
  });
  // The highlighted suggestion row; reset whenever the open set changes.
  const highlighted = signal(0);
  // Where the input's caret goes after a completion inserts text
  // (`selectedRange` controls it for that one render; typing clears it).
  const pinnedSelection = signal<{ start: number; end?: number } | null>(null);
  // The prompt-history position: null is the live draft, 0 the newest.
  const historyAt = signal<number | null>(null);
  const historyDraft = signal("");

  // The open suggestion set: derived from the value and caret in JSX so it
  // re-evaluates as typing moves the trigger (the desktop's
  // `AutocompleteView` items).
  const suggestions = (): AutocompleteItem[] => {
    const trigger = detectTrigger(props.value, selection().start);
    if (!trigger) return [];
    return filterItems(commands(), files(), chats(), trigger);
  };

  const confirmSuggestion = (item: AutocompleteItem) => {
    const trigger = detectTrigger(props.value, selection().start);
    if (!trigger) return;
    const text = itemInsertText(item);
    const value =
      props.value.slice(0, trigger.range.start) +
      text +
      props.value.slice(trigger.range.end);
    pinnedSelection.set({
      start: trigger.range.start + text.length,
    });
    highlighted.set(0);
    props.onChange(value);
  };

  const moveHighlight = (by: number) => {
    const items = suggestions();
    if (items.length === 0) return;
    highlighted.set(
      (highlighted() + by + items.length) % items.length,
    );
  };

  /// Prompt-history recall with an empty input: Up walks to older prompts,
  /// Down back to the draft. The desktop recalls the same way.
  const recallHistory = (by: number) => {
    const past = history();
    if (past.length === 0) return;
    const current = historyAt();
    let next =
      current == null
        ? by < 0
          ? past.length - 1
          : null
        : current + by;
    // Down past the oldest entry returns to the live draft.
    if (next != null && next >= past.length) next = null;
    if (next == null) {
      historyAt.set(null);
      props.onChange(historyDraft());
      pinnedSelection.set({ start: historyDraft().length });
      return;
    }
    if (next < 0) return;
    if (current == null) historyDraft.set(props.value);
    historyAt.set(next);
    const text = past[next];
    pinnedSelection.set({ start: text.length });
    props.onChange(text);
  };
  const hasDraft = () =>
    props.value.trim() !== "" ||
    attachments().length > 0 ||
    comments().length > 0 ||
    annotations().length > 0;
  const approvalMode = () => props.approvalMode ?? "AlwaysAsk";
  const thinkingLevel = () => props.thinkingLevel ?? "None";
  // The picker's one-line filter and its active tab, owned by the composer
  // so they survive opening and closing the popover (the desktop's `search`
  // entity and `active_tab`).
  const search = signal("");
  const modelOpen = signal(false);
  const approvalOpen = signal(false);
  const tab = signal<PickerTab | null>(null);
  const activeTab = () =>
    tab() ??
    ({ kind: "provider", name: props.providers?.[0]?.name ?? "" } as PickerTab);

  const send = () => {
    if (hasDraft() && !isRunning()) props.onSend();
  };

  const modelLabel = () =>
    props.selectedModel
      ? formatModelName(props.selectedModel.modelId)
      : "Select Model";
  const modelProviderIcon = (): IconName =>
    props.selectedModel
      ? providerIconName(props.selectedModel.provider)
      : "Sparkle";

  return (
    <View
      class="flex flex-col bg-[#ffffff] px-5 pt-1 pb-2.5 dark:bg-[#0a0a0a]"
      testID={props.testID}
    >
      {/* The composer box: GPUI's `composer-box` — 13px radius over the
          composer surface, accent border while a run streams. */}
      <View
        class="mx-auto flex w-full max-w-[768px] flex-col rounded-[13px] border border-[#d3d1d3] bg-[#ffffff] py-2.5 dark:border-[#3c3c3c] dark:bg-[#0a0a0a]"
        style={
          isRunning() ? { borderColor: "#e2795b52" } : undefined
        }
      >
      {comments().length > 0 ? (
        <View class="flex flex-row flex-wrap gap-1.5 px-3 pb-1.5 pt-3">
          {comments().map((comment) => (
            <View class="flex-row items-center gap-1 rounded-full border border-border bg-muted px-2.5 py-1">
              <Text class="text-[11px] text-foreground">
                {comment.label}
              </Text>
              {comment.onEdit ? (
                <View onPress={comment.onEdit} role="button">
                  <Icon name="Pen" size={11} color={token("muted-foreground")} />
                </View>
              ) : null}
              {comment.onRemove ? (
                <View onPress={comment.onRemove} role="button">
                  <Icon name="X" size={11} color={token("muted-foreground")} />
                </View>
              ) : null}
            </View>
          ))}
        </View>
      ) : null}

      {annotations().length > 0 ? (
        <View class="flex flex-row flex-wrap gap-1.5 px-3 pb-1.5 pt-3">
          {annotations().map((annotation) => (
            <View class="flex-row items-center gap-1 rounded-full border border-border bg-muted px-2 py-1">
              <Text class="text-[11px] text-muted-foreground">
                {annotation.label}
              </Text>
              {annotation.onRemove ? (
                <View onPress={annotation.onRemove} role="button">
                  <Icon name="X" size={11} color={token("muted-foreground")} />
                </View>
              ) : null}
            </View>
          ))}
        </View>
      ) : null}

      {attachments().length > 0 ? (
        <View class="flex-row flex-wrap gap-2 px-3 pb-1.5 pt-3">
          {attachments().map((attachment) => (
            <View class="h-14 w-14 overflow-hidden rounded-lg border border-border">
              <Image src={attachment.url} style={{ width: 56, height: 56 }} />
            </View>
          ))}
        </View>
      ) : null}

      {(() => {
        const items = suggestions();
        if (items.length === 0) return null;
        const at = Math.min(highlighted(), items.length - 1);
        return (
          <View
            class="flex-col gap-0.5 border-b border-border p-1.5"
            testID="composer-autocomplete"
          >
            {items.slice(0, 10).map((item, index) => (
              <ComposerAutocompleteRow
                item={item}
                selected={index === at}
                onConfirm={confirmSuggestion}
              />
            ))}
            {items.length > 10 ? (
              <Text class="px-2 text-[10px] text-muted-foreground">
                {`${items.length} matches`}
              </Text>
            ) : null}
          </View>
        );
      })()}

      <Input
        multiline
        submitBehavior="submit"
        class="min-h-[36px] px-3 py-0.5 text-sm text-foreground"
        placeholder={props.placeholder ?? "Ask Console anything…"}
        value={props.value}
        selectedRange={pinnedSelection() ?? undefined}
        ref={(ref: InputRef | null) => {
          inputRef = ref;
        }}
        onChange={(e: { value: string }) => {
          pinnedSelection.set(null);
          highlighted.set(0);
          historyAt.set(null);
          props.onChange(e.value);
        }}
        onSelectionChange={(e: { start: number; end: number }) =>
          selection.set(e)
        }
        onSubmit={send}
        testID="composer-input"
      />

      {(() => {
        const items = suggestions();
        if (items.length > 0) {
          return (
            <AutocompleteKeys
              onMove={moveHighlight}
              onConfirm={() => {
                const list = items;
                confirmSuggestion(list[Math.min(highlighted(), list.length - 1)]);
              }}
              onDismiss={() => selection.set({ start: 0, end: 0 })}
            />
          );
        }
        // History recall binds Up / Down while the field is empty or a
        // recall is in progress, so the keys keep scrolling the transcript
        // otherwise.
        if (
          (props.value === "" || historyAt() != null) &&
          history().length > 0
        ) {
          return <HistoryKeys onRecall={recallHistory} />;
        }
        return null;
      })()}

      <View class="mt-2 flex-row items-center gap-1 px-2.5">
        <View
          class="rounded-[6px] p-[5px] hover:bg-muted"
          testID="attach-button"
          accessibilityLabel="Attach"
          role="button"
        >
          <Icon name="Plus" size={13} color={token("muted-foreground")} />
        </View>

        <Popover
          open={modelOpen()}
          onOpenChange={(e) => modelOpen.set(e.open)}
          placement="top"
          align="start"
          popupClass="rounded-[13px]"
          trigger={
            <View
              class="flex h-6 flex-row items-center gap-1.5 rounded-[7px] border border-[#d3d1d3] bg-[#ffffff] px-2.5 hover:bg-muted dark:border-[#3c3c3c] dark:bg-[#0a0a0a]"
              testID="model-chip"
              accessibilityLabel="Select model"
              role="button"
            >
              <Icon
                name={modelProviderIcon()}
                size={10.5}
                color={token("foreground")}
              />
              <Text class="text-[11.5px] text-muted-foreground">
                {modelLabel()}
              </Text>
              <Icon name="ChevronDown" size={9} color={token("muted-foreground")} />
            </View>
          }
        >
          {props.providers ? (
            <ModelDropdownMenu
              providers={props.providers}
              selected={() => props.selectedModel}
              activeTab={activeTab}
              favorites={() => props.favorites ?? new Set()}
              searchQuery={() => search()}
              onSearch={(q) => search.set(q)}
              onSelect={(provider, modelId) => {
                props.onModelSelect?.(provider, modelId);
                modelOpen.set(false);
              }}
              onFavorite={(provider, modelId) => props.onFavorite?.(provider, modelId)}
              onTab={(t) => tab.set(t)}
            />
          ) : (
            <View class="p-3">
              <Text class="text-[11px] text-muted-foreground">
                No model catalog
              </Text>
            </View>
          )}
        </Popover>

        <Popover
          open={approvalOpen()}
          onOpenChange={(e) => approvalOpen.set(e.open)}
          placement="top"
          align="start"
          popupClass="rounded-xl"
          trigger={
            <View
              class="flex h-[26px] flex-row items-center gap-1.5 rounded-[7px] border border-[#d3d1d3] bg-[#ffffff] px-2.5 hover:bg-muted dark:border-[#3c3c3c] dark:bg-[#0a0a0a]"
              testID="approval-chip"
              accessibilityLabel="Approval mode"
              role="button"
            >
              <Icon
                name={approvalModeIcon(approvalMode())}
                size={11}
                color={
                  approvalMode() === "FullAccess"
                    ? token("warning")
                    : token("muted-foreground")
                }
              />
              <Text class="text-[11.5px] text-muted-foreground">
                {approvalModeLabel(approvalMode())}
              </Text>
              <Icon name="ChevronDown" size={9} color={token("muted-foreground")} />
            </View>
          }
        >
          <ApprovalModeDropdown
            selected={approvalMode()}
            onSelect={(mode) => {
              props.onApprovalMode?.(mode);
              approvalOpen.set(false);
            }}
          />
        </Popover>

        {props.onCycleThinking ? (
          <View
            class="flex h-6 flex-row items-center gap-1.5 rounded-[7px] border border-[#d3d1d3] bg-[#ffffff] px-2.5 hover:bg-muted dark:border-[#3c3c3c] dark:bg-[#0a0a0a]"
            onPress={props.onCycleThinking}
            testID="thinking-chip"
            role="button"
          >
            <Text class="text-[11.5px] text-muted-foreground">
              {thinkingLevelLabel(thinkingLevel())}
            </Text>
            <Icon name="ChevronDown" size={9} color={token("muted-foreground")} />
          </View>
        ) : null}

        <View class="flex-1" />

        <View
          class={
            "flex h-[26px] w-[26px] flex-none items-center justify-center rounded-full " +
            (hasDraft() && !isRunning()
              ? "bg-[#f4ede5] dark:bg-[#2a2520]"
              : "bg-[#d9d7d9] dark:bg-[#2f2f2f]")
          }
          onPress={send}
          testID="send-button"
          accessibilityLabel={isRunning() ? "Stop" : "Send"}
          role="button"
        >
          <Icon
            name={isRunning() ? "Stop" : "ArrowUp"}
            size={15}
            color={
              hasDraft() && !isRunning()
                ? colorScheme() === "dark"
                  ? "#f4ede5"
                  : "#3a2b21"
                : token("muted-foreground")
            }
          />
        </View>
      </View>
      </View>
    </View>
  );
}

