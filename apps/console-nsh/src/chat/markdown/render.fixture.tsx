// The markdown renderer test fixture: one document exercising every block
// kind over `METRICS.BODY`, with a scenario button to switch to the compact
// scale so tests can observe the scale prop.
import { signal } from "nsh";
import { Text, View, mount } from "nsh";
import { MarkdownBlocks, METRICS } from "./render";
import type { Block } from "./types";
import { plain, run } from "./types";

export const compact = signal(false);

const doc: Block[] = [
  { kind: "paragraph", runs: [plain("Plain text, "), run("bold", { bold: true }), plain(" and "), run("code", { code: true }), plain(".")] },
  { kind: "heading", level: 2, runs: [plain("Section")] },
  {
    kind: "list",
    items: [
      { blocks: [{ kind: "paragraph", runs: [plain("First")] }] },
      { task: true, blocks: [{ kind: "paragraph", runs: [plain("Task")] }] },
    ],
  },
  { kind: "codeBlock", language: "Rust", code: "let x = 1; // done" },
  { kind: "blockQuote", children: [{ kind: "paragraph", runs: [plain("Quoted")] }] },
  {
    kind: "table",
    header: [[plain("Name")], [plain("Count")]],
    rows: [[[plain("alpha")], [plain("12")]]],
    align: ["left", "right"],
  },
  { kind: "rule" },
  { kind: "math", latex: "E = mc^2" },
];

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
        tree={[...doc]}
        metrics={compact() ? METRICS.COMPACT : METRICS.BODY}
      />
    </View>
  );
}

mount(Fixture);
