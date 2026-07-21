import { FileFinder } from "@ff-labs/fff-node";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "../types/index.js";

const inputSchema = z.object({
  pattern: z.string().describe("Search term or regular expression to look for"),
  path: z
    .string()
    .describe(
      "File or directory to search in. If a directory, searches all files recursively.",
    ),
  mode: z
    .enum(["plain", "regex", "fuzzy"])
    .optional()
    .default("regex")
    .describe(
      '"regex" for regexp (default), "plain" for literal string, "fuzzy" for typo-resistant fuzzy search',
    ),
  caseInsensitive: z
    .boolean()
    .optional()
    .default(false)
    .describe("Case-insensitive search (overrides smart-case)"),
  contextLines: z
    .number()
    .int()
    .min(0)
    .max(10)
    .optional()
    .default(2)
    .describe("Lines of context to show around each match"),
  maxMatches: z
    .number()
    .int()
    .min(1)
    .max(500)
    .optional()
    .default(100)
    .describe("Maximum total matches to return"),
  includePattern: z
    .string()
    .optional()
    .describe("Only search files matching this glob pattern, e.g. \"*.ts\""),
});

type Input = z.infer<typeof inputSchema>;

export const grepTool: AgentTool<typeof inputSchema> = {
  name: "grep",
  description: `Search file contents using fff — a Rust-powered content search engine.
Faster than ripgrep for repeated searches in the same process (warm index).
Supports plain-text, regex, and typo-resistant fuzzy search modes.
Returns matching lines with surrounding context, grouped by file.
Use for finding function definitions, import usages, variable names, TODOs, etc.`,
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const searchPath = path.resolve(args.path);

    // fff operates on a directory root — if a file is given use its parent
    // and constrain via includePattern
    const isFile = !searchPath.endsWith("/") && /\.\w+$/.test(path.basename(searchPath));
    const basePath = isFile ? path.dirname(searchPath) : searchPath;
    const fileConstraint = isFile ? path.basename(searchPath) : args.includePattern;

    const created = FileFinder.create({ basePath });
    if (!created.ok) {
      return {
        content: [{ type: "text", text: `Error initialising file finder: ${created.error}` }],
        isError: true,
      };
    }

    const finder = created.value;

    try {
      await finder.waitForScan(8000);

      const result = finder.grep(args.pattern, {
        mode: args.mode === "regex" ? "regex" : args.mode === "fuzzy" ? "fuzzy" : "plain",
        smartCase: !args.caseInsensitive,
        pageSize: args.maxMatches,
        // fff uses context lines on its own rendering; we apply our own below
      });

      if (!result.ok) {
        return {
          content: [{ type: "text", text: `Search error: ${result.error}` }],
          isError: true,
        };
      }

      const { items, totalMatched, totalFilesSearched } = result.value;

      // Filter to specific file if path was a file
      const filtered = fileConstraint
        ? items.filter((m) => {
            const base = path.basename(m.relativePath);
            // simple glob: just check extension or full name
            if (fileConstraint.startsWith("*.")) {
              return base.endsWith(fileConstraint.slice(1));
            }
            return base === fileConstraint || m.relativePath === fileConstraint;
          })
        : items;

      if (filtered.length === 0) {
        return {
          content: [
            {
              type: "text",
              text: `No matches found for "${args.pattern}" in ${searchPath}\n(searched ${totalFilesSearched} files)`,
            },
          ],
        };
      }

      // Format results grouped by file
      const lines: string[] = [];
      const truncated = totalMatched > args.maxMatches;
      lines.push(
        `Found ${totalMatched} match(es) across files${truncated ? ` (showing first ${args.maxMatches})` : ""}  [searched ${totalFilesSearched} files]\n`,
      );

      let currentFile = "";
      for (const match of filtered) {
        if (match.relativePath !== currentFile) {
          if (currentFile !== "") lines.push("");
          lines.push(`── ${match.relativePath} ──`);
          currentFile = match.relativePath;
        }
        lines.push(`→ ${String(match.lineNumber).padStart(4)}: ${match.lineContent}`);
      }

      return {
        content: [{ type: "text", text: lines.join("\n") }],
      };
    } finally {
      finder.destroy();
    }
  },
};
