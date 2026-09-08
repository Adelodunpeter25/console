import { z } from "zod";
import type { AgentTool } from "@/agent/src/types/index.js";
import { bashJobManager } from "./manager.js";
import { BG_MAX_TIMEOUT_MS, MAX_WAIT_MS, type BashJobSnapshot } from "./types.js";

const inputSchema = z.object({
  action: z.enum(["status", "output", "wait", "kill", "list"]).describe("Job action: status, output, wait, kill, or list."),
  jobId: z.string().optional().describe("Background job id (job_...) — required for all actions except list."),
  cursor: z.number().int().min(0).optional().describe("Stdout cursor from a previous output call."),
  limit: z.number().int().min(1).max(50_000).optional().describe("Max chars per stream (default 20000, max 50000)."),
  waitMs: z
    .number()
    .int()
    .min(1)
    .max(MAX_WAIT_MS)
    .optional()
    .describe("How long to wait when action=wait (default 10000, max 120000)."),
});

type Input = z.infer<typeof inputSchema>;

export function formatJobSnapshot(s: BashJobSnapshot): string {
  const lines = [
    `Job: ${s.jobId}`,
    `Status: ${s.status}`,
    `Command: ${s.command}`,
    `Working directory: ${s.cwd}`,
    `Started: ${s.startedAt}`,
  ];
  if (s.finishedAt) lines.push(`Finished: ${s.finishedAt}`);
  if (s.exitCode !== undefined) lines.push(`Exit code: ${s.exitCode}`);
  if (s.timedOut) lines.push(`Timed out: true (max ${BG_MAX_TIMEOUT_MS}ms)`);
  if (s.aborted) lines.push(`Killed: true`);
  return lines.join("\n");
}

function ok(text: string, isError = false) {
  return { content: [{ type: "text", text }], isError };
}

function renderOutput(jobId: string, cursor?: number, limit?: number) {
  const r = bashJobManager.output(jobId, { cursor, limit });
  const sections = [
    formatJobSnapshot(r.snapshot),
    `cursor: ${cursor ?? 0} -> nextCursor: ${r.nextCursor}`,
    `truncated: ${r.truncated}`,
    "",
    "stdout:",
    r.stdout ? r.stdout : "(no new output)",
  ];
  if (r.stderr.trim()) sections.push("", "stderr:", r.stderr);
  sections.push("", "Poll again with cursor=" + r.nextCursor + " for new output.");
  return sections.join("\n");
}

export const bashJobTool: AgentTool<typeof inputSchema> = {
  name: "bashJob",
  tier: "exec",
  description:
    "Manage background shell jobs started with bash background=true. Actions: status (running/exited/failed/killed/expired + exit code), output (poll stdout/stderr with cursor, returns nextCursor + truncated flag), wait (block up to waitMs for completion without killing), kill (terminate process tree), list (all jobs). Starting a job only confirms the process started — poll status/output to learn the exit result.",
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    try {
      switch (args.action) {
        case "list": {
          const jobs = bashJobManager.list();
          if (jobs.length === 0) return ok("No background bash jobs.");
          return ok(jobs.map(formatJobSnapshot).join("\n\n---\n\n"));
        }
        case "status": {
          if (!args.jobId) return ok('Missing jobId for action="status".', true);
          const s = bashJobManager.status(args.jobId);
          return ok(formatJobSnapshot(s), s.status === "failed" || s.status === "expired");
        }
        case "output": {
          if (!args.jobId) return ok('Missing jobId for action="output".', true);
          return ok(renderOutput(args.jobId, args.cursor, args.limit));
        }
        case "wait": {
          if (!args.jobId) return ok('Missing jobId for action="wait".', true);
          const s = await bashJobManager.wait(args.jobId, args.waitMs ?? 10_000);
          const text = renderOutput(args.jobId, args.cursor, args.limit);
          const suffix =
            s.status === "running" ? `\n\nStill running after wait. Poll again or wait longer (max ${MAX_WAIT_MS}ms).` : "";
          return ok(formatJobSnapshot(s) + "\n" + text.split("\n").slice(5).join("\n") + suffix, s.status === "failed" || s.status === "expired");
        }
        case "kill": {
          if (!args.jobId) return ok('Missing jobId for action="kill".', true);
          const s = bashJobManager.kill(args.jobId);
          return ok(formatJobSnapshot(s) + "\n\nJob killed; process tree terminated.");
        }
      }
    } catch (err) {
      return ok(err instanceof Error ? err.message : String(err), true);
    }
  },
};
