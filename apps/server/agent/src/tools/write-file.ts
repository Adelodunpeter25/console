import * as fs from "node:fs/promises";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "../types/index.js";
import { pathString } from "../service/tool-input.js";

const inputSchema = z.object({
  path: pathString("Required filesystem path to write the file to."),
  cwd: z
    .string()
    .optional()
    .describe("Base directory for relative paths. Defaults to process.cwd()."),
  content: z.string().describe("Full content to write to the file"),
  encoding: z.enum(["utf-8"]).optional().default("utf-8").describe("File encoding"),
  createDirs: z
    .boolean()
    .optional()
    .default(true)
    .describe("Automatically create any missing parent directories"),
});

type Input = z.infer<typeof inputSchema>;

export const writeFileTool: AgentTool<typeof inputSchema> = {
  name: "writeFile",
  description: `Write content to a file, fully replacing its contents.
Creates the file if it does not exist. Parent directories are created automatically.
Use this for creating new files or completely rewriting an existing file.
For targeted changes to an existing file, use editFile instead to avoid rewriting unchanged content.`,
  inputSchema,
  execute: async (args: Input, _signal?: AbortSignal): Promise<unknown> => {
    const filePath = path.resolve(args.cwd ?? process.cwd(), args.path);
    const dir = path.dirname(filePath);

    // Create parent directories if needed
    if (args.createDirs) {
      try {
        await fs.mkdir(dir, { recursive: true });
      } catch (err: unknown) {
        const error = err as NodeJS.ErrnoException;
        return {
          content: [
            {
              type: "text",
              text: `Error: Could not create parent directories for "${filePath}": ${error.message}`,
            },
          ],
          isError: true,
        };
      }
    }

    const buffer = Buffer.from(args.content, "utf-8");

    try {
      await fs.writeFile(filePath, buffer);
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException;
      if (error.code === "EACCES") {
        return {
          content: [{ type: "text", text: `Error: Permission denied writing to: ${filePath}` }],
          isError: true,
        };
      }
      if (error.code === "ENOENT") {
        return {
          content: [
            {
              type: "text",
              text: `Error: Parent directory does not exist for "${filePath}". Set createDirs: true to create it automatically.`,
            },
          ],
          isError: true,
        };
      }
      return {
        content: [{ type: "text", text: `Error writing file: ${error.message}` }],
        isError: true,
      };
    }

    const lineCount = args.content.split("\n").length;
    return {
      content: [
        {
          type: "text",
          text: `Written: ${filePath}\n  Bytes: ${buffer.length}\n  Lines: ${lineCount}`,
        },
      ],
    };
  },
};
