import * as fs from "node:fs/promises";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "../types/index.js";

const inputSchema = z.object({
  path: z.string().describe("Absolute or relative path to the file to read"),
  startLine: z
    .number()
    .int()
    .positive()
    .optional()
    .describe("First line to read (1-indexed, inclusive). Omit to read from the beginning."),
  endLine: z
    .number()
    .int()
    .positive()
    .optional()
    .describe("Last line to read (1-indexed, inclusive). Omit to read to the end."),
  encoding: z
    .enum(["utf-8", "base64"])
    .optional()
    .default("utf-8")
    .describe("File encoding. Use base64 for binary files."),
});

type Input = z.infer<typeof inputSchema>;

export const readFileTool: AgentTool<typeof inputSchema> = {
  name: "readFile",
  description: `Read the contents of a file at the given path.
Supports optional line range selection for large files.
Lines are returned with their 1-indexed line numbers prefixed (e.g. "  42: content").
Always prefer reading specific line ranges for large files to avoid wasting tokens.
For directories, use listDir instead.`,
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const filePath = path.resolve(args.path);

    let raw: Buffer;
    try {
      raw = await fs.readFile(filePath);
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException;
      if (error.code === "ENOENT") {
        return {
          content: [{ type: "text", text: `Error: File not found: ${filePath}` }],
          isError: true,
        };
      }
      if (error.code === "EISDIR") {
        return {
          content: [
            {
              type: "text",
              text: `Error: "${filePath}" is a directory. Use listDir to browse directories.`,
            },
          ],
          isError: true,
        };
      }
      if (error.code === "EACCES") {
        return {
          content: [{ type: "text", text: `Error: Permission denied reading: ${filePath}` }],
          isError: true,
        };
      }
      return {
        content: [{ type: "text", text: `Error reading file: ${error.message}` }],
        isError: true,
      };
    }

    if (args.encoding === "base64") {
      const b64 = raw.toString("base64");
      return {
        content: [
          {
            type: "text",
            text: `File: ${filePath}\nEncoding: base64\nSize: ${raw.length} bytes\n\n${b64}`,
          },
        ],
      };
    }

    const fullText = raw.toString("utf-8");
    const allLines = fullText.split("\n");
    const totalLines = allLines.length;

    const start = args.startLine ?? 1;
    const end = args.endLine ?? totalLines;

    if (start > totalLines) {
      return {
        content: [
          {
            type: "text",
            text: `Error: startLine (${start}) exceeds the total number of lines in the file (${totalLines}).`,
          },
        ],
        isError: true,
      };
    }

    if (start > end) {
      return {
        content: [
          {
            type: "text",
            text: `Error: startLine (${start}) must be less than or equal to endLine (${end}).`,
          },
        ],
        isError: true,
      };
    }

    const clampedEnd = Math.min(end, totalLines);
    const selectedLines = allLines.slice(start - 1, clampedEnd);

    const lineWidth = String(clampedEnd).length;
    const numbered = selectedLines
      .map((line, i) => {
        const lineNum = String(start + i).padStart(lineWidth, " ");
        return `${lineNum}: ${line}`;
      })
      .join("\n");

    const rangeDescription =
      start === 1 && clampedEnd === totalLines
        ? `all ${totalLines} lines`
        : `lines ${start}–${clampedEnd} of ${totalLines}`;

    const header = [
      `File: ${filePath}`,
      `Showing: ${rangeDescription}`,
      `Size: ${raw.length} bytes`,
      "",
    ].join("\n");

    return {
      content: [{ type: "text", text: header + numbered }],
    };
  },
};
