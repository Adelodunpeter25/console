/**
 * A generic async event stream / queue.
 *
 * Producers push events with push() or signal failure with fail().
 * Consumers iterate with for-await-of or await result() for the final value.
 *
 * Delivery is always queue-based: events are appended to an internal queue and
 * waiters are only used to wake a blocked consumer. Mixing direct waiter
 * delivery with a queue index previously skipped events when the producer
 * outpaced the consumer (e.g. permissionRequest frames lost during tool runs).
 */
export class EventStream<TEvent, TResult> {
  private queue: TEvent[] = [];
  private waiters: Array<(value: IteratorResult<TEvent>) => void> = [];
  private done = false;
  private error: unknown = undefined;
  private resultValue: TResult | undefined = undefined;

  private readonly isDone: (event: TEvent) => boolean;
  private readonly extractResult: (event: TEvent) => TResult;

  // Promise that resolves (or rejects) when the stream finishes
  private readonly resultPromise: Promise<TResult>;
  private resolveResult!: (value: TResult) => void;
  private rejectResult!: (err: unknown) => void;

  constructor(isDone: (event: TEvent) => boolean, extractResult: (event: TEvent) => TResult) {
    this.isDone = isDone;
    this.extractResult = extractResult;
    this.resultPromise = new Promise<TResult>((res, rej) => {
      this.resolveResult = res;
      this.rejectResult = rej;
    });
  }

  /** Push an event into the stream. Wakes any waiting consumers. */
  push(event: TEvent): void {
    if (this.done) return;

    const finishing = this.isDone(event);
    if (finishing) {
      this.done = true;
      this.resultValue = this.extractResult(event);
    }

    // Always enqueue so a slow consumer never skips frames that arrived while
    // it was processing a previous event.
    this.queue.push(event);

    if (finishing) {
      this.resolveResult(this.resultValue as TResult);
    }

    this.drainWaiters();
  }

  /** Signal a fatal error. Rejects the result promise and terminates iteration. */
  fail(err: unknown): void {
    if (this.done) return;
    this.done = true;
    this.error = err;
    this.rejectResult(err);
    // Wake any waiting consumers so they observe the error/end.
    for (const waiter of this.waiters) {
      waiter({ value: undefined as unknown as TEvent, done: true });
    }
    this.waiters = [];
  }

  /** Await the final result. Rejects if the stream was failed. */
  result(): Promise<TResult> {
    return this.resultPromise;
  }

  private takeNext(): IteratorResult<TEvent> | null {
    if (this.queue.length > 0) {
      return { value: this.queue.shift()!, done: false };
    }
    if (this.done) {
      return { value: undefined as unknown as TEvent, done: true };
    }
    return null;
  }

  private drainWaiters(): void {
    while (this.waiters.length > 0) {
      const next = this.takeNext();
      if (!next) break;
      const waiter = this.waiters.shift()!;
      waiter(next);
    }
  }

  /** Async iterator — yields every event until the done event is received. */
  [Symbol.asyncIterator](): AsyncIterator<TEvent> {
    const next = (): Promise<IteratorResult<TEvent>> => {
      if (this.error !== undefined) {
        return Promise.reject(this.error);
      }

      const immediate = this.takeNext();
      if (immediate) {
        return Promise.resolve(immediate);
      }

      // Otherwise wait for the next push or fail
      return new Promise<IteratorResult<TEvent>>((resolve, reject) => {
        this.waiters.push((result) => {
          if (this.error !== undefined) {
            reject(this.error);
            return;
          }
          resolve(result);
        });
      });
    };

    return { next };
  }
}
