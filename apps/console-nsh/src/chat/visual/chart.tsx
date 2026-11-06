// Chart frame: title, canvas, legend, axis notes, and the one-line summary.
// Ported from console-ui's `chat/visual/chart.rs`; the plot itself is drawn
// by `drawChart` (`./paint`) on a nsh Canvas.

import { Canvas, Text, View, token } from "nsh";
import { describeChart } from "./core";
import { drawChart, seriesColor } from "./paint";
import type { VisualChart } from "./types";

export function ChartBlock(props: { chart: VisualChart | (() => VisualChart) }) {
  const chart = () =>
    typeof props.chart === "function" ? props.chart() : props.chart;
  return (
    <View class="flex-col gap-[8px]">
      {chart().title != null && (
        <Text class="text-[12px] text-muted-foreground" style={{ fontWeight: "500" }}>
          {chart().title}
        </Text>
      )}
      <Canvas
        class="h-[190px] w-full"
        draw={(cx, size) => drawChart(cx, size, chart())}
      />
      {chart().series.length > 1 && (
        // Legend wraps below the plot, never over it.
        <View class="flex-row flex-wrap gap-x-[14px] gap-y-[4px]">
          {chart().series.map((s, i) => (
            <View class="flex-row items-center gap-[6px]">
              <View
                class="h-[8px] w-[8px] rounded-full"
                style={{ backgroundColor: seriesColor(i) }}
              />
              <Text class="text-[11px] text-muted-foreground">{s.name}</Text>
            </View>
          ))}
        </View>
      )}
      <AxisNotes chart={chart} />
      {/* GPUI exposed no accessibility tree, so the summary is visible
          text; nsh keeps it as the chart's readable fallback. */}
      <Text class="text-[11px]" style={{ color: token("border") }}>
        {describeChart(chart())}
      </Text>
    </View>
  );
}

function AxisNotes(props: { chart: VisualChart | (() => VisualChart) }) {
  const chart = () =>
    typeof props.chart === "function" ? props.chart() : props.chart;
  const notes: string[] = [];
  if (chart().xLabel != null) notes.push(`x: ${chart().xLabel}`);
  if (chart().yLabel != null) notes.push(`y: ${chart().yLabel}`);
  if (notes.length === 0) return null;
  return (
    <Text class="text-[11px]" style={{ color: token("ring") }}>
      {notes.join("   ")}
    </Text>
  );
}
