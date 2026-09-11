export type ProjectScript = {
  id: string;
  label: string;
  command: string;
  shortcut: string | null;
  persistent: boolean;
};

export type ProjectScriptsResult = {
  projectId: string;
  scripts: ProjectScript[];
  source: "console.toml" | "missing";
};

export type ScriptRunStatus = "running" | "succeeded" | "failed" | "stopped";

export type ScriptRun = {
  runId: string;
  projectId: string;
  scriptId: string;
  label: string;
  persistent: boolean;
  status: ScriptRunStatus;
  startedAt: string;
  endedAt: string | null;
  exitCode: number | null;
  stdout: string;
  stderr: string;
};

export type ScriptRunEvent =
  | { type: "status"; status: ScriptRunStatus }
  | { type: "output"; stream: "stdout" | "stderr"; text: string }
  | { type: "exit"; status: Exclude<ScriptRunStatus, "running">; exitCode: number | null };
