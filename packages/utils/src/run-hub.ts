/**
 * Run Event Hub — per-run fan-out for live SSE subscribers.
 *
 * The agent loop produces events through a single consumer (RunService's pump).
 * This hub broadcasts each event to N attached surfaces (the initiating client,
 * a re-attaching mobile app, a desktop following along) with:
 *   - Monotonic sequence numbers (SSE `id:` field) so clients can resume.
 *   - A bounded replay buffer with coalesced modelStreamPart deltas so late
 *     joiners can catch up without receiving thousands of token frames.
 *   - Gap-free attach: subscribe() snapshots the buffer and registers the live
 *     subscriber inside one synchronous section, the same section broadcast()
 *     uses to enqueue — an event is either in the snapshot or delivered live,
 *     never both, never neither.
 *   - Idle heartbeats (raw SSE comments) so intermediaries don't reap silent
 *     connections while tools execute.
 *
 * Synthetic frames ("done"/"aborted" terminal signals) are only delivered to
 * subscribers that opt in via `extendedFrames`. The legacy POST /run pipe keeps
 * its exact wire behavior (stream just closes), so existing consumers are
 * unaffected until they adopt the new frames.
 */
import type { AgentSessionEvent } from "@console/types";

/** Maximum buffered events retained per active run for re-attach replay. */
export const MAX_REPLAY_BUFFER = 500;
/** How often the hub checks for idle connections needing a heartbeat. */
export const HEARTBEAT_CHECK_INTERVAL_MS = 5_000;
/** A connection idle longer than this gets a ping comment. */
export const HEARTBEAT_IDLE_THRESHOLD_MS = 15_000;

export interface RunStreamSubscriber {
  id: string;
  /**
   * Deliver one sequenced event. Throwing detaches the subscriber (e.g. the
   * underlying socket died) and drops its queued backlog.
   */
  deliver(seq: number, event: AgentSessionEvent): Promise<void> | void;
  /** Optional raw-channel liveness ping (SSE comment line). */
  ping?(): Promise<void> | void;
  /**
   * When true the hub also delivers synthetic terminal frames ("done"/
   * "aborted"). Legacy POST /run subscribers leave this off.
   */
  extendedFrames?: boolean;
}

interface BufferEntry {
  seq: number;
  event: AgentSessionEvent;
  /** True when this entry accumulates coalesced modelStreamPart deltas. */
  merged?: boolean;
}

interface QueuedSubscriber {
  sub: RunStreamSubscriber;
  queue: BufferEntry[];
  draining: boolean;
  dead: boolean;
  /** Resolves when the current drain loop finishes (or immediately if not draining). */
  drainPromise: Promise<void>;
}

function isSyntheticFrame(event: AgentSessionEvent): boolean {
  return event.type === "done" || event.type === "aborted";
}

export class RunEventHub {
  private seqCounter = 0;
  private buffer: BufferEntry[] = [];
  private subs = new Map<string, QueuedSubscriber>();
  private lastBroadcastAt = Date.now();
  private heartbeatTimer: ReturnType<typeof setInterval>;
  readonly settled: Promise<void>;
  private resolveSettle!: () => void;

  constructor() {
    this.settled = new Promise<void>((resolve) => {
      this.resolveSettle = resolve;
    });
    this.heartbeatTimer = setInterval(() => this.pumpHeartbeat(), HEARTBEAT_CHECK_INTERVAL_MS);
  }

  /** Broadcast one event to all live subscribers. Returns its assigned seq. */
  broadcast(event: AgentSessionEvent): number {
    const seq = ++this.seqCounter;
    this.pushToBuffer(seq, event);
    this.lastBroadcastAt = Date.now();

    const synthetic = isSyntheticFrame(event);
    const entry: BufferEntry = { seq, event };
    for (const qs of [...this.subs.values()]) {
      if (qs.dead) continue;
      if (synthetic && !qs.sub.extendedFrames) continue;
      qs.queue.push(entry);
      void this.drain(qs);
    }
    return seq;
  }

  /**
   * Attach a subscriber. When `since` is provided, buffered events with
   * seq > since are queued ahead of live delivery. Snapshot + registration
   * happen synchronously relative to any broadcast, so there is no window
   * where an event could be missed or double-delivered.
   */
  subscribe(sub: RunStreamSubscriber, since?: number): void {
    const qs: QueuedSubscriber = { sub, queue: [], draining: false, dead: false, drainPromise: Promise.resolve() };

    if (since !== undefined) {
      for (const item of this.buffer) {
        if (item.seq > since) qs.queue.push(item);
      }
    }
    this.subs.set(sub.id, qs);
    // Kick delivery of the replay backlog immediately.
    void this.drain(qs);
  }

  unsubscribe(subId: string): void {
    const qs = this.subs.get(subId);
    if (!qs) return;
    qs.dead = true;
    this.subs.delete(subId);
  }

  /** Number of live subscribers (exposed for tests/diagnostics). */
  get subscriberCount(): number {
    let count = 0;
    for (const qs of this.subs.values()) if (!qs.dead) count++;
    return count;
  }

  /** Buffered entries with seq > since (exposed for tests/diagnostics). */
  bufferedSince(since: number): { seq: number; event: AgentSessionEvent }[] {
    return this.buffer.filter((item) => item.seq > since);
  }

  /**
   * Flush all in-flight drain queues, then tear down.
   *
   * Must be awaited so the final "done"/"aborted" broadcast that ran just
   * before destroy() is called has a chance to fully deliver to every
   * subscriber before their queues are cleared and the settled promise resolves.
   */
  async destroy(): Promise<void> {
    clearInterval(this.heartbeatTimer);
    // Collect the live drain promises before we mark anything dead so we wait
    // for the exact set of drains that are currently in-flight.
    const drainPromises = [...this.subs.values()]
      .filter((qs) => !qs.dead && qs.draining)
      .map((qs) => qs.drainPromise);
    if (drainPromises.length > 0) {
      await Promise.allSettled(drainPromises);
    }
    for (const qs of this.subs.values()) qs.dead = true;
    this.subs.clear();
    this.resolveSettle();
  }

  private pushToBuffer(seq: number, event: AgentSessionEvent): void {
    // Coalesce consecutive streaming text/thinking deltas into a single
    // accumulating snapshot entry so replay stays cheap even after minutes of
    // token-by-token output. Entries carrying toolCall previews are kept as
    // discrete frames since merging them would lose intermediate previews.
    if (
      event.type === "modelStreamPart" &&
      event.part.toolCall === undefined
    ) {
      const last = this.buffer[this.buffer.length - 1];
      if (last?.merged && last.event.type === "modelStreamPart") {
        last.event.part.text = (last.event.part.text ?? "") + (event.part.text ?? "");
        last.event.part.thinking = (last.event.part.thinking ?? "") + (event.part.thinking ?? "");
        last.seq = seq;
        return;
      }
      this.buffer.push({
        seq,
        merged: true,
        event: {
          type: "modelStreamPart",
          part: { text: event.part.text ?? "", thinking: event.part.thinking ?? "" },
        },
      });
      this.trimBuffer();
      return;
    }

    this.buffer.push({ seq, event });
    this.trimBuffer();
  }

  private trimBuffer(): void {
    while (this.buffer.length > MAX_REPLAY_BUFFER) this.buffer.shift();
  }

  private drain(qs: QueuedSubscriber): Promise<void> {
    if (qs.draining || qs.dead) return qs.drainPromise;
    qs.draining = true;
    qs.drainPromise = (async () => {
      try {
        while (qs.queue.length > 0 && !qs.dead) {
          const item = qs.queue.shift()!;
          await qs.sub.deliver(item.seq, item.event);
        }
      } catch {
        // Socket write failed — detach so later broadcasts skip this surface.
        this.detach(qs);
      } finally {
        qs.draining = false;
      }
    })();
    return qs.drainPromise;
  }

  private detach(qs: QueuedSubscriber): void {
    qs.dead = true;
    this.subs.delete(qs.sub.id);
  }

  private pumpHeartbeat(): void {
    if (Date.now() - this.lastBroadcastAt < HEARTBEAT_IDLE_THRESHOLD_MS) return;
    // Treat the ping itself as activity so we send one ping per idle window,
    // not one per check interval.
    this.lastBroadcastAt = Date.now();
    for (const qs of [...this.subs.values()]) {
      if (qs.dead || !qs.sub.ping) continue;
      try {
        void Promise.resolve(qs.sub.ping()).catch(() => this.detach(qs));
      } catch {
        this.detach(qs);
      }
    }
  }
}
