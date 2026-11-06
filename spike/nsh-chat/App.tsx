// Console — phase 0 spike. All UI is TSX; the Rust core is imported as a
// native module. `nsh test` and `nsh run` both build this file.

import {
  keymap,
  mount,
  Scroll,
  signal,
  Text,
  View,
} from "nsh";
import { Input, TextView } from "nsh/base";
import { Session } from "nsh-chat/native";

type Msg = { id: number; role: "user" | "assistant"; text: string };

// Top-level signals survive hot reload.
const messages = signal<Msg[]>([
  {
    id: 1,
    role: "assistant",
    text: "Console — NSH spike. Send a message; the reply streams token by token from Rust.",
  },
]);
const draft = signal("");
const streaming = signal(false);

let nextId = 2;

function send() {
  const prompt = draft().trim();
  if (prompt === "" || streaming()) return;
  draft.set("");
  const userId = nextId++;
  const replyId = nextId++;
  messages.update((ms) => [
    ...ms,
    { id: userId, role: "user", text: prompt },
    { id: replyId, role: "assistant", text: "" },
  ]);
  streaming.set(true);
  Session.send(prompt, (token) => {
    messages.update((ms) =>
      ms.map((m) => (m.id === replyId ? { ...m, text: m.text + token } : m)),
    );
  }).then((chars) => {
    streaming.set(false);
    void chars;
  });
}

function App() {
  keymap.bind("mod+return", send, { when: "always" });

  return (
    <View class="flex flex-1 flex-col bg-background">
      <View class="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
        <Text class="text-sm font-semibold text-foreground">
          Console — NSH spike
        </Text>
        <View class="flex-1" />
        <Text class="text-xs text-muted-foreground">
          {streaming() ? "streaming…" : "idle"}
        </Text>
      </View>

      <Scroll class="flex-1">
        <View class="flex flex-col gap-3 px-4 py-4">
          {messages().map((m) =>
            m.role === "user" ? (
              <View class="w-full flex flex-row justify-end">
                <View class="max-w-[520px] rounded-2xl rounded-br-md bg-primary px-3.5 py-2">
                  <Text class="text-sm text-primary-foreground">{m.text}</Text>
                </View>
              </View>
            ) : (
              <View class="w-full flex flex-row justify-start">
                <View class="max-w-[560px] rounded-2xl rounded-bl-md bg-muted px-3.5 py-2">
                  <TextView
                    markdown={m.text}
                    options={{ streaming: true }}
                  />
                </View>
              </View>
            ),
          )}
        </View>
      </Scroll>

      <View class="flex shrink-0 flex-row items-center gap-2 border-t border-border p-3">
        <Input
          testID="composer"
          submitBehavior="submit"
          class="flex-1 rounded-xl bg-muted px-3 py-2 text-sm text-foreground"
          placeholder="Message the spike…"
          value={draft}
          onChange={(e) => draft.set(e.value)}
          onSubmit={send}
        />
        <View
          class="rounded-xl bg-primary px-4 py-2.5"
          onPress={send}
          role="button"
        >
          <Text class="text-sm font-medium text-primary-foreground">
            Send
          </Text>
        </View>
      </View>
    </View>
  );
}

mount(App);
