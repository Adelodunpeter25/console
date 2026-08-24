import * as fs from "node:fs/promises";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "@/agent/src/types/index.js";
import { pathString } from "@/agent/src/service/tool-input.js";

const inputSchema = z.object({
  path: pathString("Required filesystem path to the file to edit."),
  cwd: z
    .string()
    .optional()
    .describe("Base directory for relative paths. Defaults to process.cwd()."),
  oldContent: z
    .string()
    .describe(
      "The exact string to find in the file (including whitespace and newlines). Must match exactly once.",
    ),
  newContent: z.string().describe("The replacement string to insert in place of oldContent"),
});

type Input = z.infer<typeof inputSchema>;

export const editFileTool: AgentTool<typeof inputSchema> = {
  name: "editFile",
  description: `Edit a file by replacing one specific string with another.
The oldContent must match EXACTLY (including whitespace, indentation, and newlines).
Always call readFile first to get the exact current content before using this tool.
If the match appears 0 times or more than once, the edit is rejected — be precise.
For complete file rewrites, use writeFile instead.`,
  inputSchema,
  execute: async (args: Input, _signal?: AbortSignal): Promise<unknown> => {
    const filePath = path.resolve(args.cwd ?? process.cwd(), args.path);

    let original: string;
    try {
      original = await fs.readFile(filePath, "utf-8");
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException;
      if (error.code === "ENOENT") {
        return {
          content: [{ type: "text", text: `Error: File not found: ${filePath}` }],
          isError: true,
        };
      }
      return {
        content: [{ type: "text", text: `Error reading file: ${error.message}` }],
        isError: true,
      };
    }

    // Count occurrences of oldContent
    let occurrences = 0;
    let searchStart = 0;
    while (true) {
      const idx = original.indexOf(args.oldContent, searchStart);
      if (idx === -1) break;
      occurrences++;
      searchStart = idx + args.oldContent.length;
      if (occurrences > 1) break; // No need to count further
    }

    if (occurrences === 0) {
      // Provide a helpful hint: show the first 80 chars of what we were looking for
      const preview =
        args.oldContent.length > 80 ? args.oldContent.slice(0, 80) + "..." : args.oldContent;
      return {
        content: [
          {
            type: "text",
            text: `Error: The oldContent was not found in "${filePath}".\nLooked for:\n${preview}\n\nUse readFile to verify the current file content before editing.`,
          },
        ],
        isError: true,
      };
    }

    if (occurrences > 1) {
      return {
        content: [
          {
            type: "text",
            text:
              `Error: The oldContent appears ${occurrences} times in "${filePath}". ` +
              `Include more surrounding context in oldContent to make it unique.`,
          },
        ],
        isError: true,
      };
    }

    // Perform the replacement
    const updated = original.replace(args.oldContent, args.newContent);

    try {
      await fs.writeFile(filePath, updated, "utf-8");
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException;
      return {
        content: [{ type: "text", text: `Error writing file: ${error.message}` }],
        isError: true,
      };
    }

    // Compute a simple diff summary
    const oldLines = args.oldContent.split("\n").length;
    const newLines = args.newContent.split("\n").length;
    const lineDelta = newLines - oldLines;
    const deltaStr =
      lineDelta === 0
        ? "no line count change"
        : lineDelta > 0
          ? `+${lineDelta} lines`
          : `${lineDelta} lines`;

    return {
      content: [
        {
          type: "text",
          text: `Edited: ${filePath}\n  Replaced ${oldLines} line(s) with ${newLines} line(s) (${deltaStr})`,
        },
      ],
    };
  },
};
