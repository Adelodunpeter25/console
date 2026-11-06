// The copy button test fixture: `CopyButton` over
// `copy_button.fixture.state`, with the copy callback rendered on screen for
// the tests to observe.
import { CopyButton } from "./copy_button";
import { copiedText } from "./copy_button.fixture.state";
import { Text, View, mount } from "nsh";

function Fixture() {
  return (
    <View class="flex-row items-center gap-2 p-4">
      <CopyButton
        id="fixture-copy"
        content="copy me"
        onCopied={() => copiedText.set("copy me")}
      />
      <Text>Copied: {copiedText() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
