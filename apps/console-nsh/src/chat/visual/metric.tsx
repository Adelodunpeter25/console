// Metric tile: label, big value, optional delta with a trend arrow.
// Ported from console-ui's `chat/visual/metric.rs`.

import { Text, View, token } from "nsh";

/// The delta's tint: success green, danger red, or secondary gray, from the
/// desktop's `theme.success`/`theme.danger`/`theme.text_secondary`. nsh has
/// no success token, so the up tint takes the shared series green (the same
/// `CHART_COLORS` green the legend shows).
const TREND_GREEN = "#62C987";

export function metricTint(trend: string | null | undefined): { arrow: string; tint: string } {
  switch (trend) {
    case "up":
      return { arrow: "▲ ", tint: TREND_GREEN };
    case "down":
      return { arrow: "▼ ", tint: token("destructive") };
    case "flat":
      return { arrow: "▬ ", tint: token("muted-foreground") };
    default:
      return { arrow: "", tint: token("muted-foreground") };
  }
}

export function MetricTile(props: {
  label: string;
  value: string;
  delta?: string | null;
  trend?: string | null;
  caption?: string | null;
}) {
  return (
    <View class="min-w-[96px] flex-col gap-[2px]">
      <Text class="text-[12px] text-muted-foreground" style={{ fontWeight: "500" }}>
        {props.label}
      </Text>
      <Text class="text-[22px] text-foreground" style={{ fontWeight: "600" }}>
        {props.value}
      </Text>
      {props.delta != null && (
        <DeltaText delta={props.delta} trend={props.trend} />
      )}
      {props.caption != null && (
        <Text class="text-[11px]" style={{ color: token("ring") }}>
          {props.caption}
        </Text>
      )}
    </View>
  );
}

function DeltaText(props: { delta: string; trend?: string | null }) {
  // Arrow glyph plus color so the trend is never color-only.
  const { arrow, tint } = metricTint(props.trend);
  return (
    <Text class="text-[12px]" style={{ fontWeight: "500", color: tint }}>
      {`${arrow}${props.delta}`}
    </Text>
  );
}
