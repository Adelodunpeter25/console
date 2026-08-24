/**
 * Environment / workstation info for the system prompt.
 */
import { execFile } from "node:child_process";
import * as os from "node:os";
import * as path from "node:path";
import { promisify } from "node:util";
import type { EnvironmentInfo } from "@/agent/src/types/system-prompt.js";

const execFileAsync = promisify(execFile);

function formatLocalDate(date = new Date()): string {
  // YYYY-MM-DD in local timezone
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

async function getGitBranch(cwd: string): Promise<string | undefined> {
  try {
    const { stdout } = await execFileAsync("git", ["rev-parse", "--abbrev-ref", "HEAD"], {
      cwd,
      timeout: 2000,
      maxBuffer: 1024,
    });
    const branch = stdout.trim();
    return branch && branch !== "HEAD" ? branch : undefined;
  } catch {
    return undefined;
  }
}

export async function collectEnvironmentInfo(options: {
  cwd: string;
  model?: string;
}): Promise<EnvironmentInfo> {
  const cwd = path.resolve(options.cwd);
  const gitBranch = await getGitBranch(cwd);

  return {
    date: formatLocalDate(),
    cwd,
    os: `${os.platform()} ${os.release()}`,
    arch: os.arch(),
    node: process.version,
    gitBranch,
    model: options.model,
  };
}
