// Console — the NSH app's shell: nav rail and placeholder panes, styled from
// the theme ported from the GPUI app (`theme.json`).

import { mount, signal, Text, View } from "nsh";
import { Console } from "console-nsh/native";

type Tab = "chat" | "files" | "terminal";

const tabs: { id: Tab; label: string }[] = [
  { id: "chat", label: "Chat" },
  { id: "files", label: "Files" },
  { id: "terminal", label: "Terminal" },
];

const active = signal<Tab>("chat");
const health = signal<string | null>(null);

void checkBackend();

async function checkBackend() {
  health.set(null);
  // The Go dev server's default port; the desktop app defaults the same way.
  const body = await Console.health("http://localhost:2003");
  health.set(body);
}

function App() {
  return (
    <View class="flex flex-1 flex-row bg-background">
      <View class="flex w-48 shrink-0 flex-col border-r border-border bg-card">
        <View class="flex h-12 shrink-0 items-center px-4">
          <Text class="text-sm font-semibold text-foreground">Console</Text>
        </View>
        <View class="flex flex-col gap-1 p-2">
          {tabs.map((tab) => (
            <View
              class={
                "rounded-md px-3 py-1.5 " +
                (active() === tab.id ? "bg-muted" : "")
              }
              onPress={() => active.set(tab.id)}
              role="button"
            >
              <Text
                class={
                  "text-sm " +
                  (active() === tab.id
                    ? "font-medium text-foreground"
                    : "text-muted-foreground")
                }
              >
                {tab.label}
              </Text>
            </View>
          ))}
        </View>
      </View>

      <View class="flex flex-1 flex-col">
        <View class="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
          <Text class="text-sm text-foreground">
            {tabs.find((t) => t.id === active())?.label}
          </Text>
        </View>
        <View class="flex flex-1 flex-col items-center justify-center gap-2 p-6">
          <Text class="text-sm text-muted-foreground">
            Placeholder — panes arrive in phase 2.
          </Text>
          <Text class="text-xs text-muted-foreground">
            Backend: {health() === null ? "checking…" : health()}
          </Text>
        </View>
      </View>
    </View>
  );
}

mount(App);

export { App, checkBackend };
