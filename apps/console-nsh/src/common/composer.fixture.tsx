// The composer test fixture: the Composer wired to `composer.fixture.state`.
// `nsh test` headless runs the fixture in its own engine, so the state this
// renders on screen is how the tests observe it (signals set in the app are
// not visible to the test file's module graph).
import { Composer } from "./composer";
import {
  approval,
  draft,
  sent,
  thinking,
} from "./composer.fixture.state";
import { mount, Text, View } from "nsh";

function Fixture() {
  return (
    <View class="h-96 w-96 flex-col gap-2 p-4">
      <Composer
        value={draft()}
        onChange={(text) => draft.set(text)}
        onSend={() => sent.set(draft())}
        approvalMode={approval()}
        onApprovalMode={(m) => approval.set(m)}
        modelLabel="Claude Opus"
        thinkingLevel={thinking()}
        onCycleThinking={() => {}}
      />
      <Text class="text-xs">Sent: {sent() ?? "none"}</Text>
      <Text class="text-xs">Approval: {approval()}</Text>
      <Text class="text-xs">Draft: {draft() || "empty"}</Text>
    </View>
  );
}

mount(Fixture);
