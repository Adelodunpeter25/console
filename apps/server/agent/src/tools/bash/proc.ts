/** Subprocess-tree termination helpers (mirrors api/src/utils/exec.ts). */

export function killProcessTree(pid: number, signal: NodeJS.Signals = "SIGTERM") {
  if (process.platform === "win32") {
    try {
      Bun.spawn(["taskkill", "/pid", String(pid), "/T", "/F"], {
        stdin: "ignore",
        stdout: "ignore",
        stderr: "ignore",
      });
    } catch {}
    return;
  }
  try {
    process.kill(-pid, signal);
  } catch {}
  try {
    process.kill(pid, signal);
  } catch {}
  try {
    Bun.spawn(["pkill", `-${signal === "SIGKILL" ? "9" : "15"}`, "-P", String(pid)], {
      stdin: "ignore",
      stdout: "ignore",
      stderr: "ignore",
    });
  } catch {}
}

export function safeKill(proc: Bun.Subprocess<any, any, any>, signal?: NodeJS.Signals) {
  try {
    killProcessTree(proc.pid, signal ?? "SIGTERM");
  } catch {
    try {
      proc.kill(signal);
    } catch {}
  }
}

export function buildShellArgv(command: string): string[] {
  return process.platform === "win32"
    ? ["cmd.exe", "/d", "/s", "/c", command]
    : ["/bin/sh", "-c", command];
}

/** Fire-and-forget timers must not keep the server (or test runner) alive. */
export function unref(timer: ReturnType<typeof setTimeout> | undefined) {
  (timer as unknown as { unref?: () => void })?.unref?.();
}
