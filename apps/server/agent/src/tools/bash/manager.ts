import { randomUUID } from "node:crypto";
import { buildShellArgv, safeKill, unref } from "./proc.js";
import {
  MAX_RETAINED_JOBS,
  MAX_RUNNING_JOBS,
  MAX_OUTPUT_CHARS_PER_STREAM,
  MAX_WAIT_MS,
  RETENTION_MS,
  type BashJobSnapshot,
  type JobOutput,
  type JobOutputOptions,
  type JobRecord,
  type StartJobOptions,
} from "./types.js";

export class BashJobManager {
  private jobs = new Map<string, JobRecord>();

  get size(): number {
    return this.jobs.size;
  }

  runningCount(): number {
    let n = 0;
    for (const j of this.jobs.values()) if (j.status === "running") n++;
    return n;
  }

  start(options: StartJobOptions): JobRecord {
    if (this.runningCount() >= MAX_RUNNING_JOBS) {
      throw new Error(
        `Too many running background jobs (max ${MAX_RUNNING_JOBS}). Kill a job with bashJob action="kill" first.`,
      );
    }
    if (this.jobs.size >= MAX_RETAINED_JOBS) {
      let oldest: JobRecord | undefined;
      for (const j of this.jobs.values()) {
        if (j.status !== "running" && (!oldest || j.startedAt < oldest.startedAt)) oldest = j;
      }
      if (oldest) this.delete(oldest.jobId);
      else throw new Error(`Too many background jobs (max ${MAX_RETAINED_JOBS}).`);
    }

    let proc: Bun.Subprocess<any, any, any>;
    try {
      // `detached: true` puts the shell in its own process group, which is
      // what `process.kill(-pid, signal)` needs in proc.ts to reach the whole
      // shell pipeline (e.g. `sleep 30 | grep foo`, or a subshell-spawned
      // background sleep) — not just direct children. The pkill -P fallback
      // catches direct children but misses grandchildren without this. stdio
      // is still piped so the existing drain logic is unaffected.
      proc = Bun.spawn(buildShellArgv(options.command), {
        cwd: options.cwd,
        env: options.env as Record<string, string | undefined>,
        stdin: "ignore",
        stdout: "pipe",
        stderr: "pipe",
        detached: true,
      });
    } catch (error) {
      throw new Error(
        `Failed to start background job: ${error instanceof Error ? error.message : String(error)}`,
      );
    }

    const jobId = `job_${randomUUID().replace(/-/g, "").slice(0, 16)}`;
    const rec: JobRecord = {
      jobId,
      command: options.command,
      cwd: options.cwd,
      status: "running",
      startedAt: new Date().toISOString(),
      timedOut: false,
      aborted: false,
      proc,
      stdout: "",
      stderr: "",
      stdoutTotal: 0,
      stderrTotal: 0,
      stdoutDropped: 0,
      stderrDropped: 0,
      stdoutDone: false,
      stderrDone: false,
      exited: false,
      waiters: [],
      ownerSessionId: options.ownerSessionId ?? "default",
    };
    this.jobs.set(jobId, rec);

    rec.timer = setTimeout(() => this.expire(rec), options.timeoutMs);
    void this.drain(rec, proc.stdout as ReadableStream<Uint8Array>, "stdout");
    void this.drain(rec, proc.stderr as ReadableStream<Uint8Array>, "stderr");
    void this.trackExit(rec);
    return rec;
  }

  status(jobId: string, ownerSessionId?: string): BashJobSnapshot {
    return this.snapshot(this.getOwned(jobId, ownerSessionId));
  }

  output(jobId: string, opts: JobOutputOptions = {}): JobOutput {
    const rec = this.getOwned(jobId, opts.ownerSessionId);
    const limit = Math.min(Math.max(opts.limit ?? 20_000, 1), 50_000);
    const cursor = Math.max(opts.cursor ?? 0, 0);
    // Cursor applies to stdout; stderr is always a bounded tail.
    let truncated = false;
    let out: string;
    let nextCursor: number;
    if (cursor < rec.stdoutDropped) {
      truncated = true;
      out = rec.stdout.slice(0, limit);
      nextCursor = rec.stdoutDropped + Math.min(rec.stdout.length, limit);
    } else {
      const start = cursor - rec.stdoutDropped;
      out = rec.stdout.slice(start, start + limit);
      nextCursor = cursor + out.length;
      if (cursor > rec.stdoutTotal) nextCursor = rec.stdoutTotal;
    }
    const stderrTail =
      rec.stderr.length > limit ? rec.stderr.slice(rec.stderr.length - limit) : rec.stderr;
    if (rec.stdoutDropped > 0 || rec.stderrDropped > 0) truncated = true;
    return { snapshot: this.snapshot(rec), stdout: out, stderr: stderrTail, nextCursor, truncated };
  }

  async wait(jobId: string, waitMs: number, ownerSessionId?: string): Promise<BashJobSnapshot> {
    const rec = this.getOwned(jobId, ownerSessionId);
    if (rec.status !== "running") return this.snapshot(rec);
    const capped = Math.min(Math.max(waitMs, 1), MAX_WAIT_MS);
    await new Promise<void>((resolve) => {
      const t = setTimeout(() => {
        rec.waiters = rec.waiters.filter((w) => w !== done);
        resolve();
      }, capped);
      const done = () => {
        clearTimeout(t);
        resolve();
      };
      rec.waiters.push(done);
    });
    return this.snapshot(rec);
  }

  kill(jobId: string, ownerSessionId?: string): BashJobSnapshot {
    const rec = this.getOwned(jobId, ownerSessionId);
    if (rec.status !== "running") return this.snapshot(rec);
    safeKill(rec.proc, "SIGTERM");
    rec.escalation = setTimeout(() => safeKill(rec.proc, "SIGKILL"), 300);
    unref(rec.escalation);
    if (rec.timer) clearTimeout(rec.timer);
    rec.status = "killed";
    rec.aborted = true;
    rec.finishedAt = new Date().toISOString();
    this.scheduleRetention(rec);
    this.notify(rec);
    return this.snapshot(rec);
  }

  list(ownerSessionId?: string): BashJobSnapshot[] {
    const requester = ownerSessionId ?? "default";
    return [...this.jobs.values()]
      .filter((j) => j.ownerSessionId === requester)
      .map((j) => this.snapshot(j));
  }

  /** Terminate all running jobs (server shutdown). */
  killAll() {
    for (const rec of this.jobs.values()) {
      if (rec.status === "running") {
        try {
          safeKill(rec.proc, "SIGKILL");
        } catch {}
        if (rec.timer) clearTimeout(rec.timer);
        rec.status = "killed";
        rec.aborted = true;
        rec.finishedAt = new Date().toISOString();
        this.notify(rec);
      }
    }
  }

  /** Test-only: drop all state and kill running jobs. */
  resetForTests() {
    for (const rec of this.jobs.values()) {
      try {
        safeKill(rec.proc, "SIGKILL");
      } catch {}
      if (rec.timer) clearTimeout(rec.timer);
      if (rec.escalation) clearTimeout(rec.escalation);
      if (rec.retentionTimer) clearTimeout(rec.retentionTimer);
    }
    this.jobs.clear();
  }

  private append(rec: JobRecord, stream: "stdout" | "stderr", text: string) {
    if (!text) return;
    if (stream === "stdout") {
      rec.stdout += text;
      rec.stdoutTotal += text.length;
      if (rec.stdout.length > MAX_OUTPUT_CHARS_PER_STREAM) {
        const excess = rec.stdout.length - MAX_OUTPUT_CHARS_PER_STREAM;
        rec.stdout = rec.stdout.slice(excess);
        rec.stdoutDropped += excess;
      }
    } else {
      rec.stderr += text;
      rec.stderrTotal += text.length;
      if (rec.stderr.length > MAX_OUTPUT_CHARS_PER_STREAM) {
        const excess = rec.stderr.length - MAX_OUTPUT_CHARS_PER_STREAM;
        rec.stderr = rec.stderr.slice(excess);
        rec.stderrDropped += excess;
      }
    }
  }

  private async drain(rec: JobRecord, stream: ReadableStream<Uint8Array>, which: "stdout" | "stderr") {
    const decoder = new TextDecoder();
    const reader = stream.getReader();
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        this.append(rec, which, decoder.decode(value, { stream: true }));
      }
      this.append(rec, which, decoder.decode());
    } catch {
      // Killed mid-read — treat as done.
    } finally {
      try {
        reader.releaseLock();
      } catch {}
      if (which === "stdout") rec.stdoutDone = true;
      else rec.stderrDone = true;
      this.maybeFinish(rec);
    }
  }

  private async trackExit(rec: JobRecord) {
    try {
      rec.rawExit = await rec.proc.exited;
    } catch {
      rec.rawExit = 1;
    }
    rec.exited = true;
    this.maybeFinish(rec);
  }

  private maybeFinish(rec: JobRecord) {
    if (rec.status !== "running") return;
    if (!rec.exited || !rec.stdoutDone || !rec.stderrDone) return;
    if (rec.timer) clearTimeout(rec.timer);
    if (rec.escalation) clearTimeout(rec.escalation);
    const code = typeof rec.rawExit === "number" ? rec.rawExit : 1;
    rec.exitCode = code;
    rec.finishedAt = new Date().toISOString();
    rec.status = code === 0 ? "exited" : "failed";
    this.scheduleRetention(rec);
    this.notify(rec);
  }

  private expire(rec: JobRecord) {
    if (rec.status !== "running") return;
    rec.timedOut = true;
    safeKill(rec.proc, "SIGTERM");
    rec.escalation = setTimeout(() => safeKill(rec.proc, "SIGKILL"), 300);
    unref(rec.escalation);
    rec.status = "expired";
    rec.finishedAt = new Date().toISOString();
    if (rec.timer) clearTimeout(rec.timer);
    this.scheduleRetention(rec);
    this.notify(rec);
    const fallback = setTimeout(() => safeKill(rec.proc, "SIGKILL"), 1000);
    unref(fallback);
  }

  private scheduleRetention(rec: JobRecord) {
    if (rec.retentionTimer) clearTimeout(rec.retentionTimer);
    rec.retentionTimer = setTimeout(() => this.delete(rec.jobId), RETENTION_MS);
    unref(rec.retentionTimer);
  }

  private notify(rec: JobRecord) {
    const waiters = rec.waiters.splice(0);
    for (const w of waiters) w();
  }

  private getOwned(jobId: string, ownerSessionId?: string): JobRecord {
    const rec = this.jobs.get(jobId);
    if (!rec) throw new Error(`Background job "${jobId}" not found or expired.`);
    const requester = ownerSessionId ?? "default";
    if (rec.ownerSessionId !== requester) {
      throw new Error(`Background job "${jobId}" not found or expired.`);
    }
    return rec;
  }

  private delete(jobId: string) {
    const rec = this.jobs.get(jobId);
    if (!rec) return;
    if (rec.status === "running") return;
    if (rec.retentionTimer) clearTimeout(rec.retentionTimer);
    this.jobs.delete(jobId);
  }

  private snapshot(rec: JobRecord): BashJobSnapshot {
    return {
      jobId: rec.jobId,
      command: rec.command,
      cwd: rec.cwd,
      status: rec.status,
      exitCode: rec.exitCode,
      signal: rec.signal,
      startedAt: rec.startedAt,
      finishedAt: rec.finishedAt,
      timedOut: rec.timedOut,
      aborted: rec.aborted,
    };
  }
}

/** Process-wide singleton used by the bash / bashJob tools. */
export const bashJobManager = new BashJobManager();
