// The queued prompts fixture: a stack of two prompts over local signals,
// with the callbacks' effects rendered on screen for the tests to observe.
import { QueuedPromptsStack } from "./queued_prompt_card";
import type { QueuedPrompt } from "./queued_prompt_card";
import { Text, View, mount, signal } from "nsh";

const lastAction = signal("");

const PROMPTS: QueuedPrompt[] = [
  {
    id: "q1",
    prompt: "Fix the login loop",
    attachments: [],
    annotations: [{ selector: "login-form", componentName: "LoginForm" }],
  },
  {
    id: "q2",
    prompt:
      "This one has a very long single line of prompt text that should be cut off after ninety characters by the preview",
    attachments: [{ index: 0, url: "https://example.invalid/a.png" }, { index: 1, url: "https://example.invalid/b.png" }],
    annotations: [],
  },
];

function Fixture() {
  return (
    <View class="w-full p-2">
      <QueuedPromptsStack
        prompts={PROMPTS}
        onEdit={(id) => lastAction.set(`edit:${id}`)}
        onDelete={(id) => lastAction.set(`delete:${id}`)}
        onSteer={(id) => lastAction.set(`steer:${id}`)}
      />
      <Text>Action: {lastAction() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
