/**
 * A generic async event stream / queue.
 *
 * Producers push events with push() or signal failure with fail().
 * Consumers iterate with for-await-of or await result() for the final value.
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

  constructor(
    isDone: (event: TEvent) => boolean,
    extractResult: (event: TEvent) => TResult,
  ) {
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

    if (this.isDone(event)) {
      this.done = true;
      this.resultValue = this.extractResult(event);
      // Deliver this final event to any waiting consumers first
      if (this.waiters.length > 0) {
        const waiter = this.waiters.shift()!;
        waiter({ value: event, done: false });
      } else {
        this.queue.push(event);
      }
      // Resolve the result promise
      this.resolveResult(this.resultValue);
    } else {
      if (this.waiters.length > 0) {
        const waiter = this.waiters.shift()!;
        waiter({ value: event, done: false });
      } else {
        this.queue.push(event);
      }
    }
  }

  /** Signal a fatal error. Rejects the result promise and terminates iteration. */
  fail(err: unknown): void {
    if (this.done) return;
    this.done = true;
    this.error = err;
    this.rejectResult(err);
    // Wake any waiting consumers with the error
    for (const waiter of this.waiters) {
      waiter({ value: undefined as unknown as TEvent, done: true });
    }
    this.waiters = [];
  }

  /** Await the final result. Rejects if the stream was failed. */
  result(): Promise<TResult> {
    return this.resultPromise;
  }

  /** Async iterator — yields every event until the done event is received. */
  [Symbol.asyncIterator](): AsyncIterator<TEvent> {
    let localIndex = 0;

    const next = (): Promise<IteratorResult<TEvent>> => {
      // If there's an error, throw
      if (this.error !== undefined) {
        return Promise.reject(this.error);
      }

      // If there's something in the queue at the current index, deliver it
      if (localIndex < this.queue.length) {
        const event = this.queue[localIndex++]!;
        const isDone = this.isDone(event);
        return Promise.resolve({ value: event, done: false });
      }

      // If the stream is done and the queue is exhausted, end iteration
      if (this.done) {
        return Promise.resolve({ value: undefined as unknown as TEvent, done: true });
      }

      // Otherwise wait for the next push or fail
      return new Promise<IteratorResult<TEvent>>((resolve, reject) => {
        const wrappedResolver = (result: IteratorResult<TEvent>) => {
          if (this.error !== undefined) {
            reject(this.error);
            return;
          }
          localIndex++;
          resolve(result);
        };
        this.waiters.push(wrappedResolver);
      });
    };

    return { next };
  }
}
