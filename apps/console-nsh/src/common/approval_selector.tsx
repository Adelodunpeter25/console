// The approval-mode dropdown, ported from console-ui's
// `common/approval_selector.rs`: the icon/label presentation of the domain
// ApprovalMode plus the card the composer's approval chip opens. The enum
// itself stays a domain value (it mirrors console-core's `ApprovalMode`);
// presentation lives here.

import { Text, View, token } from "nsh";
import { Icon, type IconName } from "./icons";

export const APPROVAL_MODES = [
  "AlwaysAsk",
  "AcceptEdits",
  "PlanMode",
  "FullAccess",
] as const;
export type ApprovalMode = (typeof APPROVAL_MODES)[number];

/// console-core's `ApprovalMode::value` (the wire value).
export function approvalModeValue(mode: ApprovalMode): string {
  switch (mode) {
    case "AlwaysAsk":
      return "always-ask";
    case "AcceptEdits":
      return "accept-edits";
    case "PlanMode":
      return "plan-mode";
    case "FullAccess":
      return "full-access";
  }
}

export function approvalModeFromValue(val: string): ApprovalMode {
  switch (val) {
    case "accept-edits":
      return "AcceptEdits";
    case "plan-mode":
      return "PlanMode";
    case "full-access":
      return "FullAccess";
    default:
      return "AlwaysAsk";
  }
}

/// console-core's `ApprovalMode::label` (the chip and dropdown titles).
export function approvalModeLabel(mode: ApprovalMode): string {
  switch (mode) {
    case "AlwaysAsk":
      return "Normal";
    case "AcceptEdits":
      return "Accept Edits";
    case "PlanMode":
      return "Plan Mode";
    case "FullAccess":
      return "Full access";
  }
}

/// console-core's `ApprovalMode::description` (the dropdown subtitles).
export function approvalModeDescription(mode: ApprovalMode): string {
  switch (mode) {
    case "AlwaysAsk":
      return "Always ask for confirmation on edit & execute actions";
    case "AcceptEdits":
      return "Auto-approve safe edits, confirm shell execution";
    case "PlanMode":
      return "Read-only mode, no file edits or command execution";
    case "FullAccess":
      return "Full autonomy, auto-approve all tools";
  }
}

/// The desktop's `ApprovalModeIconExt::icon`.
export function approvalModeIcon(mode: ApprovalMode): IconName {
  switch (mode) {
    case "AlwaysAsk":
      return "TriangleAlert";
    case "AcceptEdits":
      return "Pencil";
    case "PlanMode":
      return "List";
    case "FullAccess":
      return "Zap";
  }
}

/// The card the approval chip opens (`ApprovalModeDropdown`): one row per
/// mode, the selected one highlighted with a trailing check.
export function ApprovalModeDropdown(props: {
  selected: ApprovalMode;
  onSelect: (mode: ApprovalMode) => void;
}) {
  return (
    <View class="w-[270px] flex-col overflow-hidden rounded-xl border border-border bg-popover">
      <View class="flex-col gap-0.5 p-1.5">
        {APPROVAL_MODES.map((mode) => {
          const isActive = mode === props.selected;
          return (
            <View
              class={
                "flex-row items-center justify-between gap-2 overflow-hidden rounded-md px-2.5 py-1.5 " +
                (isActive
                  ? "border border-primary/40 bg-primary/20"
                  : "hover:bg-muted")
              }
              onPress={() => props.onSelect(mode)}
              accessibilityLabel={approvalModeLabel(mode)}
              role="button"
            >
              <View class="flex-1 flex-col gap-0.5 overflow-hidden">
                <Text
                  class={
                    "text-xs text-foreground " +
                    (isActive ? "font-semibold" : "font-medium")
                  }
                >
                  {approvalModeLabel(mode)}
                </Text>
                <Text class="truncate text-[10px] text-muted-foreground">
                  {approvalModeDescription(mode)}
                </Text>
              </View>
              {isActive ? (
                <Icon name="Check" size={11} color={token("primary")} />
              ) : null}
            </View>
          );
        })}
      </View>
    </View>
  );
}
