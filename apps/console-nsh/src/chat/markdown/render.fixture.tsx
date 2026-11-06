// The markdown renderer test fixture: one markdown document exercising
// every construct, rendered through nsh's `TextView` at the desktop's body
// scale, with a scenario button to switch to the compact scale so tests can
// observe the metrics prop.
import { signal } from "nsh";
import { Text, View, mount } from "nsh";
import { MarkdownBlocks, METRICS } from "./render";

export const compact = signal(false);

const doc = [
  "Plain text, **bold** and `code`.",
  "## Section",
  "- First",
  "- [x] Task",
  "",
  "```rust",
  "let x = 1; // done",
  "```",
  "> Quoted",
  "",
  "| Name | Count |",
  "| --- | ---: |",
  "| alpha | 12 |",
  "---",
  "$$E = mc^2$$",
].join("\n");

function Fixture() {
  return (
    <View class="w-full p-2">
      <View
        class="mb-2 w-full flex-row"
        role="button"
        accessibilityLabel="Toggle scale"
        onPress={() => compact.set(!compact())}
      >
        <Text>{compact() ? "scale: compact" : "scale: body"}</Text>
      </View>
      <MarkdownBlocks
        source={doc}
        metrics={compact() ? METRICS.COMPACT : METRICS.BODY}
      />
    </View>
  );
}

mount(Fixture);
