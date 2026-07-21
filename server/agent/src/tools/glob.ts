import { FileFinder } from "@ff-labs/fff-node";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "../types/index.js";

const inputSchema = z.object({
  pattern: z
    .string()
    .describe(
      'Glob pattern compatible with npm `glob`. Examples: "src/**/*.ts", "**/*.json", "*.md", "{src,test}/**"',
    ),
  cwd: z
    .string()
    .optional()
    .describe("Root directory to search from. Defaults to process.cwd()."),
  maxResults: z
    .number()
    .int()
    .min(1)
    .max(1000)
    .optional()
    .default(200)
    .describe("Maximum number of results to return"),
});

type Input = z.infer<typeof inputSchema>;

export const globTool: AgentTool<typeof inputSchema> = {
  name: "glob",
  description: `Find files matching a glob pattern using fff — a high-performance Rust-powered file finder.
100% compatible with npm \`glob\` pattern syntax: *, **, ?, [abc], {a,b}.
Powered by a native background index so repeat calls are near-instant.
Common examples:
  - "src/**/*.ts"    → all TypeScript files under src/
  - "**/*.json"      → all JSON files anywhere
  - "*.md"           → Markdown files in the root
  - "{src,test}/**"  → everything under src/ or test/
Results are sorted by path.`,
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const searchRoot = path.resolve(args.cwd ?? process.cwd());

    const created = FileFinder.create({ basePath: searchRoot });
    if (!created.ok) {
      return {
        content: [{ type: "text", text: `Error initialising file finder: ${created.error}` }],
        isError: true,
      };
    }

    const finder = created.value;

    try {
      // Wait up to 8s for the initial scan
      await finder.waitForScan(8000);

      const result = finder.glob(args.pattern);
      if (!result.ok) {
        return {
          content: [{ type: "text", text: `Glob error: ${result.error}` }],
          isError: true,
        };
      }

      const { items, totalMatched } = result.value;
      const paths = items
        .slice(0, args.maxResults)
        .map((item) => item.relativePath)
        .sort();

      const truncated = totalMatched > args.maxResults;
      const header = `Found ${totalMatched} file(s) matching "${args.pattern}" in ${searchRoot}${truncated ? ` (showing first ${args.maxResults})` : ""}:\n`;

      if (paths.length === 0) {
        return {
          content: [
            {
              type: "text",
              text: `No files matched pattern "${args.pattern}" in ${searchRoot}`,
            },
          ],
        };
      }

      return {
        content: [{ type: "text", text: header + paths.join("\n") }],
      };
    } finally {
      finder.destroy();
    }
  },
};
