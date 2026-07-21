import { exec } from "node:child_process";
import * as path from "node:path";
import { promisify } from "node:util";
import { z } from "zod";
import type { AgentTool } from "../types/index.js";

const execAsync = promisify(exec);

const MAX_OUTPUT_BYTES = 50 * 1024; // 50KB per stream

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
    .max(300_000)
    .optional()
    .default(30_000)
    .describe("Timeout in milliseconds. The process is killed if it exceeds this. Default: 30s, max: 5min."),
  env: z
    .record(z.string())
    .optional()
    .describe("Additional environment variables to set for this command"),
});

type Input = z.infer<typeof inputSchema>;

function truncateOutput(output: string, maxBytes: number, label: string): string {
  const bytes = Buffer.byteLength(output, "utf-8");
  if (bytes <= maxBytes) return output;
  const truncated = Buffer.from(output).slice(0, maxBytes).toString("utf-8");
  return (
    truncated +
    `\n\n[... ${label} truncated: ${bytes} bytes total, showing first ${maxBytes} bytes ...]`
  );
}

export const bashTool: AgentTool<typeof inputSchema> = {
  name: "bash",
  description: `Execute a shell command and return its output.
Returns stdout, stderr, and exit code.
The command runs in a shell (/bin/sh -c), so pipes, redirects, and shell builtins all work.
Use for: running tests, builds, git operations, linters, package managers, file system queries.
Do NOT use for file reads/writes — use readFile, writeFile, editFile, or listDir instead.
Commands are killed after timeoutMs (default 30s, max 5min).
stdout and stderr are each capped at 50KB.`,
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const cwd = args.cwd ? path.resolve(args.cwd) : process.cwd();
    const env = args.env ? { ...process.env, ...args.env } : process.env;

    let stdout: string;
    let stderr: string;
    let exitCode: number;

    try {
      const result = await execAsync(args.command, {
        cwd,
        env,
        timeout: args.timeoutMs,
        maxBuffer: 10 * 1024 * 1024, // 10MB internal buffer before we truncate
        shell: process.platform === "win32" ? undefined : "/bin/sh",
      });
      stdout = result.stdout;
      stderr = result.stderr;
      exitCode = 0;
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException & {
        stdout?: string;
        stderr?: string;
        code?: number | string;
        killed?: boolean;
        signal?: string;
      };

      // Timeout
      if (error.killed || error.signal === "SIGTERM") {
        return {
          content: [
            {
              type: "text",
              text: [
                `Command timed out after ${args.timeoutMs}ms.`,
                `Command: ${args.command}`,
                `Working directory: ${cwd}`,
                "",
                "Partial stdout:",
                truncateOutput(error.stdout ?? "", MAX_OUTPUT_BYTES, "stdout"),
                "",
                "Partial stderr:",
                truncateOutput(error.stderr ?? "", MAX_OUTPUT_BYTES, "stderr"),
              ].join("\n"),
            },
          ],
          isError: true,
        };
      }

      stdout = error.stdout ?? "";
      stderr = error.stderr ?? "";
      exitCode = typeof error.code === "number" ? error.code : 1;
    }

    const truncatedStdout = truncateOutput(stdout, MAX_OUTPUT_BYTES, "stdout");
    const truncatedStderr = truncateOutput(stderr, MAX_OUTPUT_BYTES, "stderr");

    const sections: string[] = [
      `Exit code: ${exitCode}`,
      `Working directory: ${cwd}`,
    ];

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
      isError: exitCode !== 0,
    };
  },
};
