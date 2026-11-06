// The collapsible assistant reasoning section, ported from console-ui's
// `chat/thinking_block.rs`: a quiet "THINKING" header row with the reasoning's
// word count and a chevron, over the reasoning markdown when expanded.
//
// The caller owns the disclosure state so it survives transcript
// virtualization; this component only renders the header/content pair and
// reports the requested expanded state when the header is pressed. The
// desktop's selection machinery and streaming fade are GPUI concerns that
// nsh's reactivity replaces.

import { Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Icon } from "../common/icons";
import { MarkdownBlocks, METRICS } from "./markdown/render";

export function thinkingWordCount(text: string): number {
  return text.trim().split(/\s+/).filter((word) => word.length > 0).length;
}

export function ThinkingBlock(props: {
  id: string;
  /// The reasoning text. Pass a thunk when it can change after mount.
  text: string | (() => string);
  /// Whether the section is collapsed. Pass a thunk when it can change.
  collapsed: boolean | (() => boolean);
  /// Reports the expanded state the header asks for: the current `collapsed`
  /// flag, which is exactly the next value for `expanded` (the desktop's
  /// `on_toggle(collapsed, ..)`).
  onToggle?: (expanded: boolean) => void;
}) {
  const collapsed = () =>
    typeof props.collapsed === "function" ? props.collapsed() : props.collapsed;
  const text = () =>
    typeof props.text === "function" ? props.text() : props.text;
  const wordCount = () => thinkingWordCount(text());

  const header = (
    <View
      class="min-h-[16px] w-full flex-row items-center justify-between gap-1.5"
      role="button"
      accessibilityLabel={`thinking-header-${props.id}`}
      onPress={() => props.onToggle?.(collapsed())}
    >
      <View class="flex-row items-center gap-1.5">
        <Icon name="Sparkle" size={12} color={token("primary")} />
        {/* Reasoning is secondary content — paint it a step quieter than the
            assistant's answer text (the desktop's text_tertiary). */}
        <Text class="text-[11px] font-bold text-muted-foreground">
          THINKING
        </Text>
        {wordCount() > 0 ? (
          <Text class="text-[10.5px] text-muted-foreground">
            {`(${wordCount()} ${wordCount() === 1 ? "word" : "words"})`}
          </Text>
        ) : null}
      </View>
      <Icon
        name={collapsed() ? "ChevronDown" : "ChevronUp"}
        size={12}
        color={token("muted-foreground")}
      />
    </View>
  );

  const content: JSX.Element = (
    <MarkdownBlocks source={text()} metrics={METRICS.COMPACT} />
  );

  return (
    <View class="w-full flex-col gap-1 rounded-[9px] border border-border bg-muted px-3 py-1.5">
      {header}
      {collapsed() ? null : content}
    </View>
  );
}
