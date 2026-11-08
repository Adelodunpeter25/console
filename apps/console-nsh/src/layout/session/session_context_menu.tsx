// The sidebar rows' context menus, ported from console-ui's
// `primitives/context_menu/session_context_menu.rs`: right-click (or a held
// finger) on a session opens Open in New Window / Pin / Settle / Rename /
// Delete, and on a draft row just Delete Draft. Presentation and action
// ordering live here; the row owns the session-specific callbacks. Open/close
// placement, keyboard navigation and focus restoration are nsh `ContextMenu`'s.

import { ContextMenu, MenuSeparator, MenuItem } from "nsh/base";
import { Text, token } from "nsh";
import type { JSX } from "nsh";
import { Icon } from "../../common/icons";

function Entry(props: {
  label: string;
  icon: Parameters<typeof Icon>[0]["name"];
  destructive?: boolean;
  onSelect?: () => void;
}) {
  const color = token(props.destructive ? "destructive" : "muted-foreground");
  return (
    <MenuItem class="flex-row items-center gap-2" onSelect={props.onSelect}>
      <Icon name={props.icon} size={12} color={color} />
      <Text class="text-[13px]" style={{ color }}>
        {props.label}
      </Text>
    </MenuItem>
  );
}

export function SessionContextMenu(props: {
  pinned?: boolean;
  settled?: boolean;
  onOpenInNewWindow?: () => void;
  onTogglePin?: () => void;
  onRename?: () => void;
  onSettle?: () => void;
  onUnsettle?: () => void;
  onDelete?: () => void;
  children: JSX.Element;
}) {
  const settled = () => props.settled ?? false;
  return (
    <ContextMenu trigger={props.children} popupClass="min-w-[180px] rounded-xl border border-border bg-popover p-1 shadow-md">
      {props.onOpenInNewWindow ? (
        <Entry
          label="Open in New Window"
          icon="WindowRestore"
          onSelect={props.onOpenInNewWindow}
        />
      ) : null}
      <MenuSeparator />
      {props.onTogglePin ? (
        <Entry
          label={props.pinned ? "Unpin" : "Pin"}
          icon="Pin"
          onSelect={props.onTogglePin}
        />
      ) : null}
      {props.onRename ? (
        <Entry label="Rename" icon="Pencil" onSelect={props.onRename} />
      ) : null}
      {(settled() ? props.onUnsettle : props.onSettle) ? (
        <Entry
          label={settled() ? "Move to Active" : "Settle"}
          icon={settled() ? "ArrowUp" : "Check"}
          onSelect={settled() ? props.onUnsettle : props.onSettle}
        />
      ) : null}
      <MenuSeparator />
      {props.onDelete ? (
        <Entry
          label="Delete"
          icon="TrashBinMinimalistic"
          destructive
          onSelect={props.onDelete}
        />
      ) : null}
    </ContextMenu>
  );
}

/// Drafts only need a destructive discard action — no rename.
export function DraftContextMenu(props: {
  onDelete: () => void;
  children: JSX.Element;
}) {
  return (
    <ContextMenu trigger={props.children} popupClass="min-w-[180px] rounded-xl border border-border bg-popover p-1 shadow-md">
      <Entry
        label="Delete Draft"
        icon="TrashBinMinimalistic"
        destructive
        onSelect={props.onDelete}
      />
    </ContextMenu>
  );
}
