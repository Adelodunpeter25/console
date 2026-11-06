// The composer test fixture: the Composer wired to `composer.fixture.state`,
// with a small static provider catalog for the model picker. `nsh test`
// headless runs the fixture in its own engine, so the state this renders on
// screen is how the tests observe it (signals set in the app are not
// visible to the test file's module graph).
import { Composer, FloatingComposer, type CatalogProvider } from "./composer";
import {
  approval,
  draft,
  selected,
  sent,
  thinking,
} from "./composer.fixture.state";
import { mount, Text, View } from "nsh";

const providers: CatalogProvider[] = [
  {
    name: "anthropic",
    displayName: "Anthropic",
    models: [
      { id: "claude-opus-5", contextWindow: 200_000 },
      { id: "claude-sonnet-5", contextWindow: 200_000 },
    ],
  },
  {
    name: "openai",
    displayName: "OpenAI",
    models: [{ id: "gpt-6", contextWindow: 1_000_000 }],
  },
];

const commands = [{ name: "plan", description: "Plan before acting" }];
const files = [
  { relativePath: "src/main.rs", absolutePath: "/repo/src/main.rs" },
  { relativePath: "docs/overview.md", absolutePath: "/repo/docs/overview.md" },
];
const chats = [{ id: "s1", title: "Fix the login loop" }];
const promptHistory = ["First prompt", "Second prompt"];

function Fixture() {
  return (
    <View class="h-96 w-96 flex-col gap-2 p-4">
      <Composer
        value={draft()}
        onChange={(text) => draft.set(text)}
        onSend={() => sent.set(draft())}
        approvalMode={approval()}
        onApprovalMode={(m) => approval.set(m)}
        providers={providers}
        selectedModel={{ provider: "anthropic", modelId: "claude-opus-5" }}
        favorites={new Set(["anthropic:claude-sonnet-5"])}
        onModelSelect={(provider, modelId) => selected.set(`${provider}:${modelId}`)}
        thinkingLevel={thinking()}
        onCycleThinking={() => {}}
        commands={commands}
        files={files}
        chats={chats}
        promptHistory={promptHistory}
      />
      <Text class="text-xs">Sent: {sent() ?? "none"}</Text>
      <Text class="text-xs">Approval: {approval()}</Text>
      <Text class="text-xs">Draft: {draft() || "empty"}</Text>
      <Text class="text-xs">Selected: {selected() || "none"}</Text>
      <View class="h-px w-full bg-border" />
      <FloatingComposer
        value={draft()}
        onChange={(text) => draft.set(text)}
        onSend={() => sent.set(draft())}
      />
      <Text class="text-xs">Floating: {draft() || "empty"}</Text>
    </View>
  );
}

mount(Fixture);
