// The workspace's empty state, ported from console-ui's
// `workspace/empty_state.rs`: shown in a pane's content area when no chat
// tab is open. A quiet, centered welcome with a single "New Chat" action.

import { Text, View, token } from "nsh";
import { Icon } from "./icons";

export function EmptyChatState(props: { onNewChat: () => void }) {
  return (
    <View class="flex w-full flex-1 flex-col items-center justify-center gap-1.5 bg-background">
      <View class="h-11 w-11 items-center justify-center rounded-xl border border-border bg-card">
        <Icon name="chat-round-line" size={22} color={token("muted-foreground")} />
      </View>
      <Text class="mt-3.5 text-xl font-medium text-foreground">
        No chat open
      </Text>
      <Text class="text-sm text-muted-foreground">
        Start a chat or pick one from the sidebar.
      </Text>
      <View
        class="mt-2 rounded-lg bg-primary px-4 py-2 active:opacity-90"
        onPress={props.onNewChat}
        role="button"
      >
        <Text class="text-sm font-medium text-primary-foreground">
          New Chat
        </Text>
      </View>
    </View>
  );
}
