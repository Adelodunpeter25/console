// The live "still working" indicator shown below the streaming transcript,
// ported from console-ui's `chat/working_indicator.rs`: three dots chasing a
// brightness wave plus a ticking elapsed-time label ("Working for 37s"). One
// 1-second pulse drives the label; the dots re-render on their own faster
// interval (~15 FPS, the desktop's cadence).

import { effect, signal, Text, untracked, View } from "nsh";
import { formatWorkingElapsed } from "../utils/time";

const DOT_PHASE_STEP = 0.18;

function normalizeTimestamp(timestamp: number): number {
  return timestamp > 10_000_000_000 ? Math.floor(timestamp / 1000) : timestamp;
}

function WorkingWaveDots() {
  // The 1s wave, redrawn at ~15 FPS like the desktop's pulse clock. The
  // interval lives in an effect that disposes with the component's owner.
  const phase = signal(0);
  effect(() => {
    // Reads go through `untracked`: the interval callback runs in the
    // effect's scope, and a tracked read + set there would look like an
    // effect changing what it reads.
    const id = setInterval(
      () => phase.set(untracked(() => (phase() + 1 / 15) % 1)),
      1000 / 15,
    );
    return () => clearInterval(id);
  });
  return (
    <View class="flex-row items-center gap-[3.5px]">
      {[0, 1, 2].map((index) => {
        // One wave, phase-offset per dot so the bright spot travels
        // left to right (`working_wave_dots`).
        const dotPhase = (phase() + 1 - index * DOT_PHASE_STEP) % 1;
        const brightness = (Math.sin(dotPhase * Math.PI * 2) + 1) / 2;
        return (
          <View
            class="h-[4.5px] w-[4.5px] flex-none rounded-full bg-primary"
            style={{ opacity: 0.25 + 0.75 * brightness }}
          />
        );
      })}
    </View>
  );
}

export function WorkingIndicator(props: {
  /** Unix seconds (or milliseconds) when the run started streaming. */
  startedAt: number;
}) {
  const startedAt = normalizeTimestamp(props.startedAt);
  // The label recomputes its elapsed seconds once a second.
  const now = signal(Math.floor(Date.now() / 1000));
  effect(() => {
    const id = setInterval(
      () => now.set(untracked(() => Math.floor(Date.now() / 1000))),
      1000,
    );
    return () => clearInterval(id);
  });
  const seconds = () => Math.max(0, now() - startedAt);
  return (
    <View class="flex-row items-center gap-[7px] py-1.5">
    <WorkingWaveDots />
    <Text class="text-[11.5px] text-muted-foreground">
      {`Working for ${formatWorkingElapsed(seconds())}`}
    </Text>
</View>
  );
}
