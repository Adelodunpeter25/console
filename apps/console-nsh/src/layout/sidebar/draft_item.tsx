// One draft row of the sidebar, ported from console-ui's
// `layout/sidebar/draft_item.rs`. A draft of the composer that is not yet a
// chat; the "new chat" draft (no session yet) opens a new chat on press.

import { Text, View } from "nsh";
import { formatTimeAgo } from "../../utils/time";
import { DraftContextMenu } from "../session/session_context_menu";

/// What the sidebar shows for a draft; mirrors console-ui's `DraftSummary`.
export type DraftSummary = {
  sessionId: string | null;
  title: string;
  preview: string;
  projectName: string | null;
  updatedAt: number;
};

export function DraftItem(props: {
  draft: DraftSummary;
  active: boolean;
  onSelect?: (sessionId: string) => void;
  onNewChat?: () => void;
  onDelete?: () => void;
}) {
  const press = () => {
    if (props.draft.sessionId) {
      props.onSelect?.(props.draft.sessionId);
    } else {
      props.onNewChat?.();
    }
  };
  const row = (
    <View
      class={
        "flex-row items-center gap-2 rounded-md px-3 py-1.5 " +
        (props.active ? "bg-muted" : "active:bg-muted")
      }
      onPress={press}
      role="button"
    >
      <View class="flex-1 flex-col">
        <Text class="text-sm text-foreground">{props.draft.title}</Text>
        {props.draft.preview ? (
          <Text class="text-xs text-muted-foreground">
            {props.draft.preview}
          </Text>
        ) : null}
      </View>
      <Text class="text-xs text-muted-foreground">
        {formatTimeAgo(props.draft.updatedAt)}
      </Text>
    </View>
  );
  return props.onDelete ? (
    <DraftContextMenu onDelete={props.onDelete}>{row}</DraftContextMenu>
  ) : (
    row
  );
}
