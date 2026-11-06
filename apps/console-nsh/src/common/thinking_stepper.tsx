// The compact click-to-cycle thinking-level stepper, ported from console-ui's
// `common/thinking_stepper.rs`: an ultra-compact pill (`[ ✦ 3/5 ]`) inside
// the composer toolbar. Pressing cycles through the model's supported
// thinking levels in a loop; the tooltip shows the current level's label.

import { Text, View, token } from "nsh";
import type { ColorValue } from "nsh";
import { Icon } from "./icons";
import { Hint } from "./tooltip";
import type { ThinkingLevel } from "./composer";

/// Highlight colors for the brain icon across thinking levels, from the
/// desktop's `thinking_level_color` (dark-theme values; nsh's token set has
/// no per-level colors).
const LEVEL_COLORS: Record<ThinkingLevel, string> = {
  None: "#71717a",
  Minimal: "#2DD4BF", // mint / teal
  Low: "#38BDF8", // sky blue
  Medium: "#818CF8", // indigo
  High: "#C084FC", // violet / purple
  XHigh: "#F472B6", // fuchsia / magenta
  Max: "#FBBF24", // warm amber / gold
};

/// The pill's step label: the desktop's — "0/6" while off when the level
/// list starts at None, "3/5" as a 1-based index otherwise.
export function thinkingStepLabel(
  current: ThinkingLevel,
  supportedLevels: ThinkingLevel[],
): string {
  const total = supportedLevels.length;
  const index = supportedLevels.indexOf(current);
  const currentIndex = index === -1 ? 0 : index;
  if (supportedLevels[0] === "None") {
    return current === "None"
      ? `0/${total - 1}`
      : `${currentIndex}/${total - 1}`;
  }
  return `${currentIndex + 1}/${total}`;
}

export function ThinkingStepper(props: {
  currentLevel: ThinkingLevel | null | (() => ThinkingLevel | null);
  supportedLevels: ThinkingLevel[] | (() => ThinkingLevel[]);
  onCycle: () => void;
}) {
  const currentLevel = () =>
    typeof props.currentLevel === "function"
      ? props.currentLevel()
      : props.currentLevel;
  const supportedLevels = () =>
    typeof props.supportedLevels === "function"
      ? props.supportedLevels()
      : props.supportedLevels;

  const level = (): ThinkingLevel =>
    currentLevel() ?? supportedLevels()[0];
  const isOff = () => level() === "None";

  if (supportedLevels().length === 0) {
    return null;
  }

  const iconColor: ColorValue = LEVEL_COLORS[level()];

  return (
    <View
      class="h-[22px] flex-row items-center gap-[3.5px] rounded border border-border bg-card px-[5px] hover:bg-accent active:opacity-85"
      role="button"
      accessibilityLabel="Thinking level"
      tooltip={<Hint>{level()}</Hint>}
      onPress={props.onCycle}
    >
      <Icon name="Brain" size={11} color={iconColor} />
      <Text
        class="text-[10.5px] font-medium"
        style={{ color: isOff() ? token("muted-foreground") : token("foreground") }}
      >
        {thinkingStepLabel(level(), supportedLevels())}
      </Text>
    </View>
  );
}
