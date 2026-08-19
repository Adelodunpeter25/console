/**
 * read-skill Tool — On-demand skill content loader.
 *
 * The system prompt lists skills by name + description only. When the model
 * decides a skill is relevant to the current task, it calls this tool with the
 * skill name to pull the full skill content + file path into context.
 *
 * This keeps the system prompt minimal (cache-friendly) while still giving the
 * agent access to the complete skill workflow when it matters.
 */
import { z } from "zod";
import type { AgentTool } from "../types/index.js";
import { discoverSkills } from "../systemprompt/discover-skills.js";

const inputSchema = z.object({
  name: z
    .string()
    .describe(
      "Exact name of the skill to read (as listed in the system prompt). " +
        "Omit or leave empty to list every available skill with its description.",
    )
    .optional(),
  cwd: z
    .string()
    .optional()
    .describe("Base directory for project-level skill discovery. Defaults to process.cwd()."),
});

type Input = z.infer<typeof inputSchema>;

export const readSkillTool: AgentTool<typeof inputSchema> = {
  name: "readSkill",
  description: `Read the full content of a skill by name.
The system prompt only lists skill names with one-line descriptions.
Call this tool when a skill matches the current task to load its complete instructions, file path, and workflow details.
If called without a name, returns the list of all available skills.`,
  tier: "read",
  inputSchema,
  execute: async (args: Input, _signal?: AbortSignal): Promise<unknown> => {
    const cwd = args.cwd ?? process.cwd();
    const skills = await discoverSkills({ cwd });

    // No name given — return the catalog of available skills.
    const query = args.name?.trim();
    if (!query) {
      if (skills.length === 0) {
        return {
          content: [
            {
              type: "text",
              text: "No skills discovered. Skills are loaded from `skills/` directories under `.console`, `.agent`, or `.agents` in the project tree and user home.",
            },
          ],
        };
      }
      const lines = skills.map((s) => {
        const desc = s.description?.trim() || "(no description)";
        return `- ${s.name}: ${desc}`;
      });
      return {
        content: [
          {
            type: "text",
            text: `Available skills (${skills.length}):\n${lines.join("\n")}\n\nCall readSkill with a name to load full content.`,
          },
        ],
      };
    }

    // Case-insensitive match on skill name.
    const matched = skills.find((s) => s.name.toLowerCase() === query.toLowerCase());

    if (!matched) {
      const available = skills.map((s) => s.name).join(", ");
      const hint = available
        ? `\n\nAvailable skills: ${available}`
        : "\n\nNo skills were discovered.";
      return {
        content: [
          {
            type: "text",
            text: `Skill '${query}' not found.${hint}`,
          },
        ],
        isError: true,
      };
    }

    const header = [
      `Skill: ${matched.name}`,
      `File: ${matched.path}`,
      "",
    ].join("\n");

    return {
      content: [
        {
          type: "text",
          text: header + matched.content,
        },
      ],
    };
  },
};
