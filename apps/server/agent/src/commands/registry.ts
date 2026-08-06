/**
 * SlashCommandRegistry — manages built-in and discovered custom slash commands.
 */
import type { Agent } from "../service/agent.js";
import type { SqliteSessionStorage } from "../session/storage.js";
import type { Model, Skill, SlashCommandFile } from "../types/index.js";
import {
  compactCommand,
  modeCommand,
  modelCommand,
  newSessionCommand,
  providerCommand,
  renameSessionCommand,
  resumeSessionCommand,
} from "./builtins.js";

export interface SlashCommandContext {
  agent: Agent;
  sessionStorage?: SqliteSessionStorage;
  currentSessionId?: string;
  currentProvider: "gemini" | "antigravity" | "opencode";
  setProvider: (provider: "gemini" | "antigravity" | "opencode") => void;
  setModel: (model: Model) => void;
  setCurrentSessionId: (id: string) => void;
  discoveredCommands?: SlashCommandFile[];
  discoveredSkills?: Skill[];
}

export interface SlashCommandResult {
  handled: boolean;
  message?: string;
  action?:
    | "clear"
    | "new"
    | "resume"
    | "switch_model"
    | "switch_provider"
    | "compact"
    | "custom_prompt";
  customPromptText?: string;
}

export interface SlashCommand {
  name: string;
  description: string;
  execute: (
    args: string,
    ctx: SlashCommandContext,
  ) => Promise<SlashCommandResult> | SlashCommandResult;
}

export class SlashCommandRegistry {
  private readonly commands = new Map<string, SlashCommand>();

  constructor() {
    this.registerBuiltins();
  }

  private registerBuiltins(): void {
    this.register(modelCommand);
    this.register(providerCommand);
    this.register(modeCommand);
    this.register(newSessionCommand);
    this.register(resumeSessionCommand);
    this.register(renameSessionCommand);
    this.register(compactCommand);

    // Help command
    this.register({
      name: "help",
      description: "List all available slash commands",
      execute: (_args, ctx) => {
        const builtinList = Array.from(this.commands.values()).map(
          (c) => `  /${c.name.padEnd(12)} - ${c.description}`,
        );

        const customCommands = ctx.discoveredCommands ?? [];
        const customList = customCommands.map(
          (c) => `  /${c.name.padEnd(12)} - ${c.description || "custom user command"}`,
        );

        const skillCommands = (ctx.discoveredSkills ?? [])
          .filter((s) => !s.hide)
          .map((s) => `  /${s.name.padEnd(12)} - ${s.description || "load skill context"}`);

        const sections = ["Available Slash Commands:", ...builtinList];

        if (customList.length > 0) {
          sections.push("", "Discovered Custom Commands:", ...customList);
        }

        if (skillCommands.length > 0) {
          sections.push("", "Discovered Skills:", ...skillCommands);
        }

        return { handled: true, message: sections.join("\n") };
      },
    });
  }

  /**
   * Register a new SlashCommand.
   */
  register(command: SlashCommand): void {
    this.commands.set(command.name.toLowerCase(), command);
  }

  /**
   * List all registered built-in commands.
   */
  getCommands(): SlashCommand[] {
    return Array.from(this.commands.values());
  }

  /**
   * Parse user input for a slash command (`/command [args]`).
   * Returns `{ handled: true, ... }` if a command was matched and executed,
   * or `{ handled: false }` if input should be processed normally as a prompt.
   */
  async parseAndExecute(input: string, ctx: SlashCommandContext): Promise<SlashCommandResult> {
    const trimmed = input.trim();
    if (!trimmed.startsWith("/")) {
      return { handled: false };
    }

    const spaceIndex = trimmed.indexOf(" ");
    const cmdName = (
      spaceIndex === -1 ? trimmed.slice(1) : trimmed.slice(1, spaceIndex)
    ).toLowerCase();
    const args = spaceIndex === -1 ? "" : trimmed.slice(spaceIndex + 1).trim();

    // 1. Check built-in commands
    const builtin = this.commands.get(cmdName);
    if (builtin) {
      return builtin.execute(args, ctx);
    }

    // 2. Check discovered custom commands (.agent/commands/*.md)
    if (ctx.discoveredCommands?.length) {
      const matchedCmd = ctx.discoveredCommands.find((c) => c.name.toLowerCase() === cmdName);
      if (matchedCmd) {
        const promptText = args
          ? `${matchedCmd.content}\n\nAdditional user input:\n${args}`
          : matchedCmd.content;
        return {
          handled: true,
          action: "custom_prompt",
          customPromptText: promptText,
          message: `Invoked custom slash command /${cmdName}`,
        };
      }
    }

    // 3. Check discovered skills (/skill:<name> or /<skillName>)
    if (ctx.discoveredSkills?.length) {
      const cleanSkillName = cmdName.startsWith("skill:") ? cmdName.slice(6) : cmdName;
      const matchedSkill = ctx.discoveredSkills.find(
        (s) => s.name.toLowerCase() === cleanSkillName.toLowerCase(),
      );
      if (matchedSkill) {
        const skillText = `[Skill Instructions: ${matchedSkill.name}]\nFile: ${matchedSkill.path}\n\n${matchedSkill.content}${args ? `\n\nUser request:\n${args}` : ""}`;
        return {
          handled: true,
          action: "custom_prompt",
          customPromptText: skillText,
          message: `Loaded skill /${matchedSkill.name}`,
        };
      }
    }

    return {
      handled: true,
      message: `Unknown command '/${cmdName}'. Type /help to view available commands.`,
    };
  }
}
