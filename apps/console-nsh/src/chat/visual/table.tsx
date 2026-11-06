// Summary table. Numeric-looking columns right-align; wide tables scroll
// horizontally inside the card, never the transcript. Ported from
// console-ui's `chat/visual/table.rs`.

import { Scroll, Text, View, token } from "nsh";
import { looksNumeric } from "./core";

export function VisualTable(props: {
  caption?: string | null;
  columns: string[];
  rows: string[][];
}) {
  // Cells read in table order, header then row by row, one cell per line
  // when copied.
  const cell = (text: string, right: boolean, header: boolean) => (
    <View
      class={
        "min-w-[84px] px-[8px] py-[6px]" +
        (right ? " flex-row justify-end" : "")
      }
    >
      <Text
        class="text-[12px]"
        style={{
          fontWeight: header ? "500" : "400",
          color: header ? token("muted-foreground") : token("foreground"),
        }}
      >
        {text}
      </Text>
    </View>
  );
  const numericCols = () =>
    props.columns.map((_, c) => {
      return (
        props.rows.length > 0 &&
        props.rows.every((r) => r[c] != null && looksNumeric(r[c]))
      );
    });
  return (
    <View class="flex-col gap-[6px]">
      {props.caption != null && (
        <Text class="text-[12px] text-muted-foreground" style={{ fontWeight: "500" }}>
          {props.caption}
        </Text>
      )}
      <Scroll horizontal class="w-full flex-col">
        <View class="w-full flex-row border-b border-border">
          {props.columns.map((name, c) => cell(name, numericCols()[c], true))}
        </View>
        {props.rows.map((row) => (
          <View class="w-full flex-row">
            {row.map((v, c) => cell(v, numericCols()[c] ?? false, false))}
          </View>
        ))}
      </Scroll>
    </View>
  );
}
