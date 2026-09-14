/**
 * Built-in `/init` slash command — generate a `console.toml` for the project.
 *
 * This is the only built-in slash command. It carries no server-side execution;
 * like user-defined commands it is prompt-level: the listing surfaces in
 * autocomplete (`assist.ts`) and the system prompt (`builder.ts`), and the
 * agent does the work with its normal file tools.
 */

export const INIT_COMMAND_NAME = "init";

export const INIT_COMMAND_DESCRIPTION =
  "Generate a console.toml with project run scripts for the Run tab";

/**
 * Short usage + schema reference inlined under `/init` in the system prompt,
 * so the agent knows the `console.toml` format without a tool round-trip.
 * Mirrors the validation rules in `api/src/services/project-scripts/config.ts`.
 */
export const INIT_COMMAND_DETAILS = [
  "Usage: /init — inspect package.json, Makefile, and other project manifests, then write console.toml at the project root.",
  "Schema: one [scripts.<id>] table per script; id matches /^[A-Za-z0-9_-]+$/.",
  "Required per script: label (non-empty), command (non-empty, runs with the repo root as working directory).",
  "Optional per script: shortcut (e.g. cmd-shift-r), persistent (boolean, true for long-running dev servers).",
  "Rules: never overwrite an existing console.toml without asking; validate the result parses before finishing.",
  "If the project has no console.toml, suggest /init or offer to create one.",
].join(" ");

/**
 * Match a user prompt that invokes `/init` — leading whitespace allowed,
 * `/init` alone or followed by whitespace/args. Case-sensitive on purpose:
 * only lowercase `/init` is the command.
 */
export function isInitPrompt(prompt: string): boolean {
  return /^\/init(?:\s|$)/.test(prompt.trimStart());
}
