import * as path from "node:path";
import { z } from "zod";
import { spawnCapture } from "@/api/src/utils/exec.js";
import type { AgentTool } from "@/agent/src/types/index.js";
import { bashJobManager } from "./manager.js";
import { BG_DEFAULT_TIMEOUT_MS } from "./types.js";

const MAX_OUTPUT_BYTES = 50 * 1024;
const DEFAULT_TIMEOUT_MS = 30_000;
const MAX_TIMEOUT_MS = 20 * 60 * 1000;

const inputSchema = z.object({
  command: z.string().describe("Shell command to execute"),
  cwd: z.string().optional().describe("Working directory for the command. Defaults to process.cwd()."),
  timeoutMs: z
    .number()
    .int()
    .min(1000)
    .max(MAX_TIMEOUT_MS)
    .optional()
    .describe(
      "Timeout in milliseconds. Sync mode: time the tool call waits (default 30s). Background mode: max job lifetime (default 10min). Max: 20min.",
    ),
  env: z.record(z.string()).optional().describe("Additional environment variables for this command"),
  background: z
    .boolean()
    .optional()
    .describe(
      "Start as a background job and return immediately with a jobId. Poll with bashJob (status/output/wait/kill). The start response is NOT an exit result.",
    ),
});

type Input = z.infer<typeof inputSchema>;

function truncateOutput(output: string, maxBytes: number, label: string): string {
  const bytes = Buffer.byteLength(output, "utf-8");
  if (bytes <= maxBytes) return output;
  const truncated = Buffer.from(output).slice(0, maxBytes).toString("utf-8");
  return truncated + `\n\n[... ${label} truncated: ${bytes} bytes total, showing first ${maxBytes} bytes ...]`;
}

export const bashTool: AgentTool<typeof inputSchema> = {
  name: "bash",
  tier: "exec",
  description:
    "Run a shell command and return output. Use for builds, tests, or git; prefer dedicated tools for reading/editing files. For long-running commands (builds), pass background=true to start a managed job and poll it with bashJob — the start response only confirms the process started, not success.",
  inputSchema,
  execute: async (args: Input, signal?: AbortSignal): Promise<unknown> => {
    const cwd = args.cwd ? path.resolve(args.cwd) : process.cwd();
    const env = args.env ? { ...process.env, ...args.env } : process.env;
    if (args.background) {
      const timeoutMs = args.timeoutMs ?? BG_DEFAULT_TIMEOUT_MS;
      try {
        const rec = bashJobManager.start({ command: args.command, cwd, env, timeoutMs });
        return {
          content: [
            {
              type: "text",
              text: [
                `Started background bash job ${rec.jobId}.`,
                `Status: running (NOT a success result — poll for the exit code).`,
                `Command: ${args.command}`,
                `Working directory: ${cwd}`,
                `Max lifetime: ${timeoutMs}ms`,
                "",
                `Next: bashJob action="output" jobId="${rec.jobId}" for output,`,
                `bashJob action="wait" jobId="${rec.jobId}" to block until done,`,
                `bashJob action="kill" jobId="${rec.jobId}" to terminate.`,
              ].join("\n"),
            },
          ],
        };
      } catch (err) {
        return {
          content: [{ type: "text", text: err instanceof Error ? err.message : String(err) }],
          isError: true,
        };
      }
    }
    const timeoutMs = args.timeoutMs ?? DEFAULT_TIMEOUT_MS;
    const argv =
      process.platform === "win32" ? ["cmd.exe", "/d", "/s", "/c", args.command] : ["/bin/sh", "-c", args.command];
    const result = await spawnCapture(argv, { cwd, env, timeoutMs, signal });

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
    if (result.killed) {
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
    sections.push(truncatedStdout.trim() ? "" : "", truncatedStdout.trim() ? "stdout:" : "stdout: (empty)");
    if (truncatedStdout.trim()) sections.push(truncatedStdout);
    if (truncatedStderr.trim()) sections.push("", "stderr:", truncatedStderr);
    return { content: [{ type: "text", text: sections.join("\n") }], isError: result.exitCode !== 0 };
  },
};
