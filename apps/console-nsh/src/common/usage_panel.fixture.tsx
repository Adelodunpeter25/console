// The usage panel test fixture: the `UsageMeter` (its trigger opens the
// panel) over local signals. The test cannot reach the fixture's module
// graph, so the fixture renders a "Seed" button that populates the state
// in-app; the shape line shows what the meter received.
import { UsageMeter } from "./usage_meter";
import type { ContextSnapshot, UsageReport } from "./usage_meter";
import { Text, View, mount, signal } from "nsh";

const report = signal<UsageReport | null>(null);
const snapshot = signal<ContextSnapshot | null>(null);
const loading = signal(false);

function Fixture() {
  return (
    <View class="h-64 w-96 items-end justify-start gap-1 p-4">
      <UsageMeter
        usageReport={() => report()}
        contextSnapshot={() => snapshot()}
        isLoading={() => loading()}
      />
      <View class="flex-row gap-2">
        <View
          class="rounded px-1 hover:bg-muted"
          role="button"
          accessibilityLabel="Seed"
          onPress={() => {
            loading.set(false);
            report.set({
              limits: [
                {
                  label: "Five hour window",
                  status: "ok",
                  window: { resetLabel: "resets in 3h 20m" },
                  amount: { used: 90, limit: 300, unit: "minutes" },
                },
              ],
            });
            snapshot.set({
              usedTokens: 23_100,
              contextWindow: 1_000_000,
              percentUsed: 2.3,
              thresholdRatio: 0.9,
            });
          }}
        >
          <Text class="text-xs text-foreground">Seed</Text>
        </View>
        <View
          class="rounded px-1 hover:bg-muted"
          role="button"
          accessibilityLabel="Start loading"
          onPress={() => loading.set(true)}
        >
          <Text class="text-xs text-foreground">Load</Text>
        </View>
      </View>
      <Text>
        Shape:{" "}
        {report()
          ? `${report()!.limits.length} limits`
          : loading()
            ? "loading"
            : "none"}
      </Text>
    </View>
  );
}

mount(Fixture);
