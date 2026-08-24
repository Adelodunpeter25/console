/**
 * Shared subprocess runner built on `Bun.spawn` (replaces the Node
 * `child_process.exec` shim). Captures stdout/stderr as UTF-8 with a hard
 * byte cap — unlike the shim, overflow never silently kills data mid-stream:
 * we keep draining the pipe (discarding excess) so the child can finish and
 * report a real exit code.
 */

const DEFAULT_MAX_BYTES = 10 * 1024 * 1024;

export interface SpawnCaptureOptions {
  cwd: string;
  env?: Record<string, string | undefined>;
  /** Kill the child with SIGTERM (then SIGKILL) after this many ms. */
  timeoutMs?: number;
  /** Kill the child when this fires; marks the result as aborted. */
  signal?: AbortSignal;
  /** Per-stream byte budget before capture switches to drain-and-discard. */
  maxBytes?: number;
}

export interface SpawnCaptureResult {
  stdout: string;
  stderr: string;
  exitCode: number;
  /** Terminated by our timeout or by the caller's abort signal. */
  killed: boolean;
  /** Killed specifically because the caller's abort signal fired. */
  aborted: boolean;
}

function safeKill(proc: Bun.Subprocess<any, any, any>, signal: Parameters<typeof proc.kill>[0]) {
  try {
    proc.kill(signal);
  } catch {
    // Already exited — nothing to do.
  }
}

async function readCapped(
  stream: ReadableStream<Uint8Array>,
  maxBytes: number,
): Promise<string> {
  const decoder = new TextDecoder();
  const reader = stream.getReader();
  let text = "";
  let total = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    const previousTotal = total;
    total += value.byteLength;
    if (previousTotal < maxBytes) {
      // Retain exactly up to the budget, slicing mid-chunk when it straddles
      // the boundary so small caps still keep their first bytes.
      text += decoder.decode(value.subarray(0, maxBytes - previousTotal), { stream: true });
    }
    // Over budget: keep reading (drain) but stop retaining, so the child
    // never blocks on a full pipe and still terminates normally.
  }
  return text + decoder.decode();
}

export async function spawnCapture(
  argv: string[],
  options: SpawnCaptureOptions,
): Promise<SpawnCaptureResult> {
  const maxBytes = options.maxBytes ?? DEFAULT_MAX_BYTES;

  let proc: Bun.Subprocess<any, any, any>;
  try {
    proc = Bun.spawn(argv, {
      cwd: options.cwd,
      env: options.env,
      stdin: "ignore",
      stdout: "pipe",
      stderr: "pipe",
    });
  } catch (error) {
    return {
      stdout: "",
      stderr: `[Process error: ${error instanceof Error ? error.message : String(error)}]`,
      exitCode: 1,
      killed: false,
      aborted: false,
    };
  }

  let timedOut = false;
  let abortedBySignal = false;

  let escalation: ReturnType<typeof setTimeout> | undefined;
  const terminate = () => {
    safeKill(proc, undefined);
    escalation = setTimeout(() => safeKill(proc, "SIGKILL"), 500);
  };

  const onAbort = () => {
    abortedBySignal = true;
    terminate();
  };

  if (options.signal) {
    if (options.signal.aborted) onAbort();
    else options.signal.addEventListener("abort", onAbort, { once: true });
  }

  let timer: ReturnType<typeof setTimeout> | undefined;
  if (options.timeoutMs != null) {
    timer = setTimeout(() => {
      timedOut = true;
      terminate();
    }, options.timeoutMs);
  }

  try {
    const [stdout, stderr] = await Promise.all([
      readCapped(proc.stdout as ReadableStream<Uint8Array>, maxBytes),
      readCapped(proc.stderr as ReadableStream<Uint8Array>, maxBytes),
    ]);
    const rawExit = await proc.exited;

    return {
      stdout,
      stderr,
      exitCode: typeof rawExit === "number" ? rawExit : 1,
      killed: timedOut || abortedBySignal,
      aborted: abortedBySignal,
    };
  } catch (error) {
    return {
      stdout: "",
      stderr: `[Process error: ${error instanceof Error ? error.message : String(error)}]`,
      exitCode: 1,
      killed: timedOut || abortedBySignal,
      aborted: abortedBySignal,
    };
  } finally {
    if (timer) clearTimeout(timer);
    if (escalation) clearTimeout(escalation);
    if (options.signal) options.signal.removeEventListener("abort", onAbort);
  }
}

/**
 * Shell-command flavor of {@link spawnCapture} that mirrors the old
 * `promisify(exec)` contract: rejects on a non-zero exit with an error whose
 * message and `.stderr` carry the child's stderr (call sites match on that
 * text), resolving with stdout/stderr otherwise.
 */
export interface ExecShellOptions {
  cwd: string;
  timeoutMs?: number;
  signal?: AbortSignal;
  maxBytes?: number;
}

export async function execShell(
  command: string,
  options: ExecShellOptions,
): Promise<{ stdout: string; stderr: string }> {
  const result = await spawnCapture(["/bin/sh", "-c", command], options);
  if (result.exitCode !== 0) {
    const error = new Error(
      `Command failed: ${command}\n${result.stderr}`.trim(),
    ) as Error & { code?: number; stdout?: string; stderr?: string };
    error.code = result.exitCode;
    error.stdout = result.stdout;
    error.stderr = result.stderr;
    throw error;
  }
  return { stdout: result.stdout, stderr: result.stderr };
}
