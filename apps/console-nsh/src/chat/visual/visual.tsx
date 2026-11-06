// One card for a `visualize` tool result, ported from console-ui's
// `chat/visual/mod.rs`. The spec is data; these files only show it.
// Parsing, axis math, and the point-bucketing safety net live in
// `./types` and `./core` so they are testable without the app. A card
// blends into the transcript (no border, chat background); an unsupported
// or malformed spec shows a compact fallback instead of failing the row.
//
// - `visual.tsx`: the card shell, title block, and block dispatch
// - `metric.tsx`: label/value/delta tiles
// - `table.tsx`: summary table with right-aligned numeric columns
// - `chart.tsx`: chart frame, legend, axis notes, accessibility summary
// - `paint.tsx`: the canvas painter (grid, ticks, series)
// - `core.ts` / `types.ts`: the spec decode and pure geometry
//
// Skipped from the desktop port: `select.rs` — the GPUI transcript-selection
// machinery (selectable text runs keyed by tool call id). nsh text carries
// no such selection model, so the card's text is plain.

import { Text, View } from "nsh";
import { parseVisual } from "./types";
import type { VisualArtifact, VisualBlock, VisualSpec } from "./types";
import { MetricTile } from "./metric";
import { VisualTable } from "./table";
import { ChartBlock } from "./chart";

export function VisualCard(props: {
  artifact: VisualArtifact | (() => VisualArtifact);
}) {
  const artifact = () =>
    typeof props.artifact === "function" ? props.artifact() : props.artifact;
  const parsed = () => parseVisual(artifact());
  return (
    <View class="w-full flex-col gap-[12px] px-[4px] py-[6px]">
      {(() => {
        const parse = parsed();
        if (parse.tag === "ok") return <CardBody spec={() => parse.spec} />;
        if (parse.tag === "unsupported") {
          return <Fallback message="This visual needs a newer version of Console." />;
        }
        return <Fallback message="Visual unavailable" />;
      })()}
    </View>
  );
}

function Fallback(props: { message: string }) {
  return (
    <Text class="text-[12.5px] text-muted-foreground" style={{ fontWeight: "500" }}>
      {props.message}
    </Text>
  );
}

function CardBody(props: { spec: VisualSpec | (() => VisualSpec) }) {
  const spec = () =>
    typeof props.spec === "function" ? props.spec() : props.spec;
  return (
    <View class="w-full flex-col gap-[12px]">
      {(spec().title != null || spec().subtitle != null) && (
        <View class="flex-col gap-[2px]">
          {spec().title != null && (
            <Text class="text-[14px] text-foreground" style={{ fontWeight: "600" }}>
              {spec().title}
            </Text>
          )}
          {spec().subtitle != null && (
            <Text class="text-[12px] text-muted-foreground" style={{ fontWeight: "500" }}>
              {spec().subtitle}
            </Text>
          )}
        </View>
      )}
      <Blocks blocks={spec().blocks} />
    </View>
  );
}

/// Consecutive metrics share a wrapping row; everything else stacks.
function Blocks(props: { blocks: VisualBlock[] }) {
  const groups: VisualBlock[][] = [];
  for (const block of props.blocks) {
    const last = groups.at(-1);
    if (block.type === "metric" && last && last[0].type === "metric") {
      last.push(block);
    } else {
      groups.push([block]);
    }
  }
  return (
    <View class="flex-col gap-[12px]">
      {groups.map((group) =>
        group[0].type === "metric" ? (
          <View class="flex-row flex-wrap gap-x-[28px] gap-y-[12px]">
            {group.map((m) =>
              m.type === "metric" ? (
                <MetricTile
                  label={m.label}
                  value={m.value}
                  delta={m.delta}
                  trend={m.trend}
                  caption={m.caption}
                />
              ) : null,
            )}
          </View>
        ) : (
          <SingleBlock block={group[0]} />
        ),
      )}
    </View>
  );
}

function SingleBlock(props: { block: VisualBlock }) {
  const block = props.block;
  if (block.type === "text") {
    return (
      <Text class="text-[13px] text-muted-foreground" style={{ fontWeight: "500" }}>
        {block.text}
      </Text>
    );
  }
  if (block.type === "table") {
    return (
      <VisualTable caption={block.caption} columns={block.columns} rows={block.rows} />
    );
  }
  if (block.type === "chart") {
    return <ChartBlock chart={block.chart} />;
  }
  return null;
}
