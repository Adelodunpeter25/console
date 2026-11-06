// Fixture for the visual card: a full spec over every block kind, with
// scenario buttons to switch to an unsupported (newer) version and an
// invalid artifact so tests can observe the fallbacks.
import { signal } from "nsh";
import { Text, View, mount } from "nsh";
import { VisualCard } from "./visual";
import type { VisualArtifact } from "./types";

const full: VisualArtifact = {
  version: 1,
  spec: {
    version: 1,
    title: "Latency",
    subtitle: "p50 by day",
    blocks: [
      { type: "text", text: "Weekly rollout health." },
      {
        type: "metric",
        label: "p50",
        value: "128ms",
        delta: "-12%",
        trend: "down",
        caption: "vs last week",
      },
      {
        type: "metric",
        label: "uptime",
        value: "99.9%",
        delta: "+0.1%",
        trend: "up",
      },
      {
        type: "table",
        caption: "By region",
        columns: ["Region", "Requests", "Errors"],
        rows: [
          ["us-east", "12,400", "18"],
          ["eu-west", "9,120", "7"],
        ],
      },
      {
        type: "chart",
        kind: "line",
        title: "p50 over days",
        xLabel: "day",
        yLabel: "ms",
        series: [
          {
            name: "p50",
            points: [
              { x: 0, y: 140, label: "Mon" },
              { x: 1, y: 132, label: "Tue" },
              { x: 2, y: 128, label: "Wed" },
              { x: 3, y: 118, label: "Thu" },
            ],
          },
          {
            name: "p95",
            points: [
              { x: 0, y: 210, label: "Mon" },
              { x: 1, y: 240, label: "Tue" },
              { x: 2, y: 230, label: "Wed" },
              { x: 3, y: 219, label: "Thu" },
            ],
          },
        ],
      },
    ],
  },
};

const newer: VisualArtifact = { version: 2, spec: { version: 2, blocks: [] } };
const invalid: VisualArtifact = { version: 1, spec: { version: 1, blocks: [{ type: "chart" }] } };

const scenarios = { full, newer, invalid } as Record<
  string,
  VisualArtifact
>;
export const scenario = signal<VisualArtifact>(full);

function Fixture() {
  return (
    <View class="w-full flex-col gap-2 p-2">
      <View
        class="w-full flex-row"
        role="button"
        accessibilityLabel="Show unsupported"
        onPress={() => scenario.set(newer)}
      >
        <Text>scenario: unsupported</Text>
      </View>
      <View
        class="w-full flex-row"
        role="button"
        accessibilityLabel="Show invalid"
        onPress={() => scenario.set(invalid)}
      >
        <Text>scenario: invalid</Text>
      </View>
      <VisualCard artifact={() => scenario()} />
    </View>
  );
}

mount(Fixture);