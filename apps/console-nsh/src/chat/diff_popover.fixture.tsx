// Fixture for the diff popover: one chip carrying a diff longer than the
// preview cap, and one binary chip with no stored diff, over a local signal
// showing what "open the full diff" reported.
import { WithDiffPopover } from "./diff_popover";
import { Text, View, mount, signal } from "nsh";

const lastAction = signal("");

const header = "diff --git a/big.ts b/big.ts";
const longDiff =
  header +
  Array.from(
    { length: 430 },
    (_, i) => `\n@@ -${i + 1},1 +${i + 1},1 @@\n+line ${i + 1}`,
  ).join("");

const bigChip = (
  <View
    class="flex-row items-center gap-1.5 rounded-[7px] border border-border px-2 py-[3px]"
    role="button"
    accessibilityLabel="Show diff for big.ts"
  >
    <Text class="text-[12px] text-muted-foreground">big.ts</Text>
  </View>
);

const imageChip = (
  <View
    class="flex-row items-center gap-1.5 rounded-[7px] border border-border px-2 py-[3px]"
    role="button"
    accessibilityLabel="Show diff for logo.png"
  >
    <Text class="text-[12px] text-muted-foreground">logo.png</Text>
  </View>
);

function Fixture() {
  return (
    <View class="w-full flex-row flex-wrap gap-1.5 p-2">
      <WithDiffPopover
        change={{
          path: "src/big.ts",
          status: "modified",
          additions: 430,
          deletions: 0,
          diffText: longDiff,
        }}
        trigger={bigChip}
        onOpen={(path) => lastAction.set(`open:${path}`)}
      />
      <WithDiffPopover
        change={{
          path: "assets/logo.png",
          status: "added",
          additions: 0,
          deletions: 0,
          diffText: null,
        }}
        trigger={imageChip}
        onOpen={(path) => lastAction.set(`open:${path}`)}
      />
      <Text>Action: {lastAction() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
