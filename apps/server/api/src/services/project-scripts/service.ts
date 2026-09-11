import { randomUUID } from "node:crypto";
import { getSharedSessionStorage } from "@/agent/src/session/storage.js";
import { loadProjectScripts } from "./config.js";
import type { ProjectScript, ProjectScriptsResult, ScriptRun, ScriptRunEvent, ScriptRunStatus } from "./types.js";

type Subscriber = (event: ScriptRunEvent) => void;

type ManagedRun = ScriptRun & {
  process: Bun.Subprocess<any, any, any>;
  subscribers: Set<Subscriber>;
};

const MAX_OUTPUT = 256 * 1024;

async function readOutput(stream: ReadableStream<Uint8Array>, streamName: "stdout" | "stderr", run: ManagedRun) {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      const text = decoder.decode(value, { stream: true });
      if (streamName === "stdout") run.stdout = (run.stdout + text).slice(-MAX_OUTPUT);
      else run.stderr = (run.stderr + text).slice(-MAX_OUTPUT);
      run.subscribers.forEach((subscriber) => subscriber({ type: "output", stream: streamName, text }));
    }
  } finally {
    reader.releaseLock();
  }
}

export class ProjectScriptsService {
  private readonly storage = getSharedSessionStorage();
  private readonly runs = new Map<string, ManagedRun>();

  async list(projectId: string): Promise<ProjectScriptsResult> {
    const project = this.storage.getProject(projectId);
    if (!project) throw new Error(`Project '${projectId}' not found.`);
    return loadProjectScripts(projectId, project.path);
  }

  async run(projectId: string, scriptId: string): Promise<ScriptRun> {
    const project = this.storage.getProject(projectId);
    if (!project) throw new Error(`Project '${projectId}' not found.`);
    const config = await loadProjectScripts(projectId, project.path);
    const script = config.scripts.find((candidate) => candidate.id === scriptId);
    if (!script) throw new Error(`Script '${scriptId}' not found.`);
    const existing = [...this.runs.values()].find((run) => run.projectId === projectId && run.scriptId === scriptId && run.status === "running");
    if (existing) throw new Error(`Script '${scriptId}' is already running.`);

    const child = Bun.spawn([process.platform === "win32" ? "cmd.exe" : "sh", process.platform === "win32" ? "/c" : "-c", script.command], {
      cwd: project.path,
      stdin: "ignore",
      stdout: "pipe",
      stderr: "pipe",
      detached: process.platform !== "win32",
    });
    const run: ManagedRun = {
      runId: randomUUID(), projectId, scriptId, label: script.label, persistent: script.persistent,
      status: "running", startedAt: new Date().toISOString(), endedAt: null, exitCode: null,
      stdout: "", stderr: "", process: child, subscribers: new Set(),
    };
    this.runs.set(run.runId, run);
    run.subscribers.forEach((subscriber) => subscriber({ type: "status", status: "running" }));
    void Promise.all([
      readOutput(child.stdout, "stdout", run),
      readOutput(child.stderr, "stderr", run),
      child.exited,
    ]).then(([, , exitCode]) => {
      if (run.status === "stopped") return;
      run.exitCode = exitCode;
      run.endedAt = new Date().toISOString();
      run.status = exitCode === 0 ? "succeeded" : "failed";
      run.subscribers.forEach((subscriber) => subscriber({ type: "exit", status: run.status as Exclude<ScriptRunStatus, "running">, exitCode }));
      run.subscribers.clear();
    });
    return this.snapshot(run);
  }

  listRuns(projectId: string): ScriptRun[] {
    return [...this.runs.values()].filter((run) => run.projectId === projectId).map((run) => this.snapshot(run));
  }

  getRun(projectId: string, runId: string): ScriptRun | null {
    const run = this.runs.get(runId);
    return run && run.projectId === projectId ? this.snapshot(run) : null;
  }

  stop(projectId: string, runId: string): boolean {
    const run = this.runs.get(runId);
    if (!run || run.projectId !== projectId || run.status !== "running") return false;
    try {
      if (process.platform !== "win32") process.kill(-run.process.pid, "SIGTERM");
      else run.process.kill();
    } catch {
      run.process.kill();
    }
    run.status = "stopped";
    run.endedAt = new Date().toISOString();
    run.subscribers.forEach((subscriber) => subscriber({ type: "exit", status: "stopped", exitCode: null }));
    run.subscribers.clear();
    return true;
  }

  subscribe(projectId: string, runId: string, subscriber: Subscriber): (() => void) | null {
    const run = this.runs.get(runId);
    if (!run || run.projectId !== projectId) return null;
    subscriber({ type: "status", status: run.status });
    if (run.stdout) subscriber({ type: "output", stream: "stdout", text: run.stdout });
    if (run.stderr) subscriber({ type: "output", stream: "stderr", text: run.stderr });
    if (run.status !== "running") subscriber({ type: "exit", status: run.status as Exclude<ScriptRunStatus, "running">, exitCode: run.exitCode });
    else run.subscribers.add(subscriber);
    return () => run.subscribers.delete(subscriber);
  }

  private snapshot(run: ManagedRun): ScriptRun {
    const { process: _process, subscribers: _subscribers, ...snapshot } = run;
    return snapshot;
  }
}
