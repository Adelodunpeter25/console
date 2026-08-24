import * as path from "node:path";
import { z } from "zod";
import { spawnCapture } from "../../../api/src/utils/exec.js";
import type { AgentTool } from "../types/index.js";

const MAX_OUTPUT_BYTES = 50 * 1024;
const DEFAULT_TIMEOUT_MS = 30_000;
const MAX_TIMEOUT_MS = 20 * 60 * 1000;

const inputSchema = z.object({
  command: z.string().describe("Shell command to execute"),
  cwd: z
    .string()
    .optional()
    .describe("Working directory for the command. Defaults to process.cwd()."),
  timeoutMs: z
    .number()
    .int()
    .min(1000)
    .max(MAX_TIMEOUT_MS)
    .optional()
    .describe(
      "Timeout in milliseconds. The process is killed if it exceeds this. Default: 30s, max: 20min.",
    ),
  env: z
    .record(z.string())
    .optional()
    .describe("Additional environment variables to set for this command"),
});

type Input = z.infer<typeof inputSchema>;

interface ExecResult {
  stdout: string;
  stderr: string;
  exitCode: number;
  killed: boolean;
  aborted: boolean;
}

function truncateOutput(output: string, maxBytes: number, label: string): string {
  const bytes = Buffer.byteLength(output, "utf-8");
  if (bytes <= maxBytes) return output;
  const truncated = Buffer.from(output).slice(0, maxBytes).toString("utf-8");
  return (
    truncated +
    `\n\n[... ${label} truncated: ${bytes} bytes total, showing first ${maxBytes} bytes ...]`
  );
}

async function execWithSignal(
  command: string,
  options: {
    cwd: string;
    env: NodeJS.ProcessEnv;
    timeout: number;
    signal?: AbortSignal;
  },
): Promise<ExecResult> {
  const argv =
    process.platform === "win32"
      ? ["cmd.exe", "/d", "/s", "/c", command]
      : ["/bin/sh", "-c", command];

  return spawnCapture(argv, {
    cwd: options.cwd,
    env: options.env,
    timeoutMs: options.timeout,
    signal: options.signal,
  });
}

export const bashTool: AgentTool<typeof inputSchema> = {
  name: "bash",
  tier: "exec",
  description: `Execute a shell command and return its output.
Returns stdout, stderr, and exit code.
The command runs in a shell (/bin/sh -c), so pipes, redirects, and shell builtins all work.
Use for: running tests, builds, git operations, linters, package managers, file system queries.
Do NOT use for file reads/writes — use readFile, writeFile, editFile, or listDir instead.
Commands are killed after timeoutMs (default 30s, max 20min).
stdout and stderr are each capped at 50KB.`,
  inputSchema,
  execute: async (args: Input, signal?: AbortSignal): Promise<unknown> => {
    const cwd = args.cwd ? path.resolve(args.cwd) : process.cwd();
    const env = args.env ? { ...process.env, ...args.env } : process.env;
    const timeoutMs = args.timeoutMs ?? DEFAULT_TIMEOUT_MS;

    const result = await execWithSignal(args.command, {
      cwd,
      env,
      timeout: timeoutMs,
      signal,
    });

    if (result.aborted) {
      return {
        content: [
          {
            type: "text",
            text: [
              "Command cancelled by user abort.",
              `Command: ${args.command}`,
              `Working directory: ${cwd}`,
              "",
              "Partial stdout:",
              truncateOutput(result.stdout, MAX_OUTPUT_BYTES, "stdout"),
              "",
              "Partial stderr:",
              truncateOutput(result.stderr, MAX_OUTPUT_BYTES, "stderr"),
            ].join("\n"),
          },
        ],
        isError: true,
      };
    }

    if (result.killed && !result.aborted) {
      return {
        content: [
          {
            type: "text",
            text: [
              `Command timed out after ${timeoutMs}ms.`,
              `Command: ${args.command}`,
              `Working directory: ${cwd}`,
              "",
              "Partial stdout:",
              truncateOutput(result.stdout, MAX_OUTPUT_BYTES, "stdout"),
              "",
              "Partial stderr:",
              truncateOutput(result.stderr, MAX_OUTPUT_BYTES, "stderr"),
            ].join("\n"),
          },
        ],
        isError: true,
      };
    }

    const truncatedStdout = truncateOutput(result.stdout, MAX_OUTPUT_BYTES, "stdout");
    const truncatedStderr = truncateOutput(result.stderr, MAX_OUTPUT_BYTES, "stderr");

    const sections: string[] = [`Exit code: ${result.exitCode}`, `Working directory: ${cwd}`];

    if (truncatedStdout.trim()) {
      sections.push("", "stdout:", truncatedStdout);
    } else {
      sections.push("", "stdout: (empty)");
    }

    if (truncatedStderr.trim()) {
      sections.push("", "stderr:", truncatedStderr);
    }

    return {
      content: [{ type: "text", text: sections.join("\n") }],
      isError: result.exitCode !== 0,
    };
  },
};
