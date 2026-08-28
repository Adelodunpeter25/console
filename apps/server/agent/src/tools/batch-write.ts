import * as fs from "node:fs/promises";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "@/agent/src/types/index.js";
import { pathString } from "@/agent/src/service/tool-input.js";

const fileEntrySchema = z.object({
  path: pathString("Required filesystem path to write."),
  content: z.string().describe("Full content of the file"),
});

const inputSchema = z.object({
  files: z
    .array(fileEntrySchema)
    .min(1)
    .describe(
      "List of files to write. Each entry has a path and content. All writes happen concurrently.",
    ),
  cwd: z
    .string()
    .optional()
    .describe("Base directory for relative paths. Defaults to process.cwd()."),
  stopOnError: z
    .boolean()
    .optional()
    .default(false)
    .describe(
      "If true, stop after the first failure and report which files were not written. " +
        "If false (default), attempt all writes and report each result individually.",
    ),
});

type Input = z.infer<typeof inputSchema>;

interface FileWriteResult {
  path: string;
  status: "written" | "failed";
  bytes?: number;
  lines?: number;
  error?: string;
}

async function writeOneFile(
  baseCwd: string,
  filePath: string,
  content: string,
): Promise<FileWriteResult> {
  const resolved = path.resolve(baseCwd, filePath);
  const dir = path.dirname(resolved);

  try {
    await fs.mkdir(dir, { recursive: true });
    const buffer = Buffer.from(content, "utf-8");
    await fs.writeFile(resolved, buffer);
    return {
      path: resolved,
      status: "written",
      bytes: buffer.length,
      lines: content.split("\n").length,
    };
  } catch (err: unknown) {
    const error = err as Error;
    return {
      path: resolved,
      status: "failed",
      error: error.message,
    };
  }
}

function formatResults(results: FileWriteResult[]): string {
  const lines: string[] = [];
  const written = results.filter((r) => r.status === "written");
  const failed = results.filter((r) => r.status === "failed");

  lines.push(`Summary: ${written.length}/${results.length} files written successfully.`);
  lines.push("");

  if (written.length > 0) {
    lines.push("✓ Written:");
    for (const r of written) {
      lines.push(`  ${r.path}  [${r.bytes ?? 0}B, ${r.lines ?? 0} lines]`);
    }
  }

  if (failed.length > 0) {
    lines.push("");
    lines.push("✗ Failed:");
    for (const r of failed) {
      lines.push(`  ${r.path}`);
      lines.push(`    Error: ${r.error}`);
    }
  }

  return lines.join("\n");
}

export const batchWriteTool: AgentTool<typeof inputSchema> = {
  name: "batchWrite",
  description: "Write multiple files at once. Useful for project scaffolding or multi-file creation.",
  inputSchema,
  execute: async (args: Input, _signal?: AbortSignal): Promise<unknown> => {
    const baseCwd = path.resolve(args.cwd ?? process.cwd());
    // Check for duplicate paths
    const resolvedPaths = args.files.map((f) => path.resolve(baseCwd, f.path));
    const seen = new Set<string>();
    const duplicates: string[] = [];
    for (const rp of resolvedPaths) {
      if (seen.has(rp)) duplicates.push(rp);
      else seen.add(rp);
    }
    if (duplicates.length > 0) {
      return {
        content: [
          {
            type: "text",
            text:
              `Error: Duplicate paths detected in batchWrite — each path must be unique:\n` +
              duplicates.map((p) => `  ${p}`).join("\n"),
          },
        ],
        isError: true,
      };
    }

    let results: FileWriteResult[];

    if (args.stopOnError) {
      // Sequential — stop on first error
      results = [];
      for (const file of args.files) {
        const result = await writeOneFile(baseCwd, file.path, file.content);
        results.push(result);
        if (result.status === "failed") {
          // Mark remaining as skipped
          const remaining = args.files.slice(results.length);
          for (const skipped of remaining) {
            results.push({
              path: path.resolve(baseCwd, skipped.path),
              status: "failed",
              error: "Skipped due to stopOnError",
            });
          }
          break;
        }
      }
    } else {
      // Concurrent — attempt all
      results = await Promise.all(
        args.files.map((file) => writeOneFile(baseCwd, file.path, file.content)),
      );
    }

    const anyFailed = results.some((r) => r.status === "failed");
    return {
      content: [{ type: "text", text: formatResults(results) }],
      isError: anyFailed,
    };
  },
};
