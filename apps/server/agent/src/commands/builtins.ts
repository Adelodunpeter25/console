/**
 * Built-in Slash Commands.
 * Handles /model, /provider, /new, /resume, /rename, /compact, /help.
 */
import {
  fetchModelsForProvider,
  findModelInProvider,
  getProvider,
  listProviders,
} from "./provider-registry.js";
import type { SlashCommand, SlashCommandContext, SlashCommandResult } from "./registry.js";

export const modelCommand: SlashCommand = {
  name: "model",
  description: "Switch model or list available models for the active provider",
  execute: async (args: string, ctx: SlashCommandContext): Promise<SlashCommandResult> => {
    const targetModelId = args.trim();

    // Dynamically discover live models from the provider endpoint
    const models = await fetchModelsForProvider(ctx.currentProvider);

    if (!targetModelId) {
      const activeModelId = ctx.agent.model.id;
      const lines = [
        `Available models for provider '${ctx.currentProvider}':`,
        ...models.map((m) => (m.id === activeModelId ? `  * ${m.id} (active)` : `    ${m.id}`)),
        "",
        `Use \`/model <id>\` to switch model.`,
      ];
      return { handled: true, message: lines.join("\n") };
    }

    const matched = findModelInProvider(ctx.currentProvider, targetModelId);
    if (!matched) {
      const available = models.map((m) => m.id).join(", ");
      return {
        handled: true,
        message: `Unknown model '${targetModelId}' for provider '${ctx.currentProvider}'.\nAvailable: ${available}`,
      };
    }

    ctx.setModel(matched);
    if (ctx.currentSessionId && ctx.sessionStorage) {
      ctx.sessionStorage.updateModel(ctx.currentSessionId, matched.id, matched.provider);
    }

    return {
      handled: true,
      action: "switch_model",
      message: `Switched model to: ${matched.id} (${matched.provider})`,
    };
  },
};

export const providerCommand: SlashCommand = {
  name: "provider",
  description: "Switch LLM provider (gemini vs antigravity) or list providers",
  execute: async (args: string, ctx: SlashCommandContext): Promise<SlashCommandResult> => {
    const targetProviderName = args.trim().toLowerCase();

    if (!targetProviderName) {
      const providers = listProviders();
      const lines = [
        "Available providers:",
        ...providers.map((p) =>
          p.name === ctx.currentProvider
            ? `  * ${p.name} - ${p.displayName} (active)`
            : `    ${p.name} - ${p.displayName}`,
        ),
        "",
        "Use `/provider <name>` to switch provider.",
      ];
      return { handled: true, message: lines.join("\n") };
    }

    const providerEntry = getProvider(targetProviderName);
    if (!providerEntry) {
      const available = listProviders()
        .map((p) => p.name)
        .join(", ");
      return {
        handled: true,
        message: `Unknown provider '${targetProviderName}'. Available: ${available}`,
      };
    }

    ctx.setProvider(providerEntry.name);

    // Refresh dynamic models for the new provider
    const models = await fetchModelsForProvider(providerEntry.name);
    if (models.length === 0) {
      return {
        handled: true,
        message: `Switched provider to: ${providerEntry.displayName}\nNo models are currently available — check your authentication and network connection.`,
      };
    }
    const defaultModel = models[0]!;
    ctx.setModel(defaultModel);

    if (ctx.currentSessionId && ctx.sessionStorage) {
      ctx.sessionStorage.updateModel(ctx.currentSessionId, defaultModel.id, defaultModel.provider);
    }

    return {
      handled: true,
      action: "switch_provider",
      message: `Switched provider to: ${providerEntry.displayName}\nActive model set to: ${defaultModel.id}`,
    };
  },
};

export const newSessionCommand: SlashCommand = {
  name: "new",
  description: "Start a new clean conversation session",
  execute: async (_args: string, ctx: SlashCommandContext): Promise<SlashCommandResult> => {
    ctx.agent.clearHistory();

    if (ctx.sessionStorage) {
      const newSession = ctx.sessionStorage.createSession({
        cwd: process.cwd(),
        modelId: ctx.agent.model.id,
        provider: ctx.agent.model.provider,
        title: "New Session",
      });
      ctx.setCurrentSessionId(newSession.id);
      return {
        handled: true,
        action: "new",
        message: `Started new session: ${newSession.id}`,
      };
    }

    return {
      handled: true,
      action: "clear",
      message: "Cleared conversation history.",
    };
  },
};

export const resumeSessionCommand: SlashCommand = {
  name: "resume",
  description: "Resume a saved session by ID or list recent sessions",
  execute: async (args: string, ctx: SlashCommandContext): Promise<SlashCommandResult> => {
    const targetId = args.trim();

    if (!targetId) {
      if (!ctx.sessionStorage) {
        return { handled: true, message: "Session storage is not enabled." };
      }

      const sessions = ctx.sessionStorage.listSessions({ limit: 10 });
      if (sessions.length === 0) {
        return { handled: true, message: "No saved sessions found." };
      }

      const lines = [
        "Recent sessions:",
        ...sessions.map((s) => {
          const activeMarker = s.id === ctx.currentSessionId ? " (active)" : "";
          const dateStr = new Date(s.updatedAt).toLocaleString();
          return `  - ${s.id} | "${s.title}" (${s.modelId}, ${s.messageCount ?? 0} msgs, ${dateStr})${activeMarker}`;
        }),
        "",
        "Use `/resume <session_id>` to resume a session.",
      ];
      return { handled: true, message: lines.join("\n") };
    }

    if (!ctx.sessionStorage) {
      return { handled: true, message: "Session storage is not enabled." };
    }

    const loaded = ctx.sessionStorage.loadSession(targetId);
    if (!loaded) {
      return { handled: true, message: `Session not found: ${targetId}` };
    }

    ctx.agent.clearHistory();
    ctx.agent.loadHistory(loaded.messages);
    ctx.setCurrentSessionId(loaded.header.id);

    // If model exists in catalog, update active model
    const matchedModel = findModelInProvider(loaded.header.provider, loaded.header.modelId);
    if (matchedModel) {
      ctx.setProvider(matchedModel.provider);
      ctx.setModel(matchedModel);
    }

    return {
      handled: true,
      action: "resume",
      message: `Resumed session '${loaded.header.title}' (${loaded.header.id}) with ${loaded.messages.length} messages.`,
    };
  },
};

export const renameSessionCommand: SlashCommand = {
  name: "rename",
  description: "Rename the title of the current session",
  execute: async (args: string, ctx: SlashCommandContext): Promise<SlashCommandResult> => {
    const newTitle = args.trim();
    if (!newTitle) {
      return { handled: true, message: "Usage: `/rename <new title>`" };
    }

    if (!ctx.currentSessionId || !ctx.sessionStorage) {
      return { handled: true, message: "No active saved session to rename." };
    }

    const success = ctx.sessionStorage.updateTitle(ctx.currentSessionId, newTitle);
    if (!success) {
      return { handled: true, message: "Failed to rename session." };
    }

    return {
      handled: true,
      message: `Renamed session (${ctx.currentSessionId}) to: "${newTitle}"`,
    };
  },
};

export const compactCommand: SlashCommand = {
  name: "compact",
  description: "Compact and summarize conversation history to save context",
  execute: async (_args: string, ctx: SlashCommandContext): Promise<SlashCommandResult> => {
    const messages = ctx.agent.messages;
    if (messages.length === 0) {
      return { handled: true, message: "Conversation history is empty." };
    }

    const userCount = messages.filter((m) => m.role === "user").length;
    const assistantCount = messages.filter((m) => m.role === "assistant").length;

    // Compact history into a summary checkpoint turn
    const summaryText = `[Conversation Summary: Compacted ${messages.length} messages (${userCount} user prompts, ${assistantCount} assistant turns).]`;

    ctx.agent.clearHistory();
    ctx.agent.loadHistory([
      { role: "user", content: "Summarize history check" },
      {
        role: "assistant",
        id: crypto.randomUUID(),
        content: [{ type: "text", text: summaryText }],
        stopReason: "stop",
      },
    ]);

    return {
      handled: true,
      action: "compact",
      message: `Compacted conversation history from ${messages.length} messages to 2 summary messages.`,
    };
  },
};

export const modeCommand: SlashCommand = {
  name: "mode",
  description: "Switch security approval mode (always-ask, accept-edits, plan-mode, full-access)",
  execute: async (args: string, ctx: SlashCommandContext): Promise<SlashCommandResult> => {
    const targetMode = args.trim().toLowerCase();
    const validModes = ["always-ask", "accept-edits", "plan-mode", "full-access"];

    if (!targetMode) {
      const current = ctx.agent.approvalMode;
      const lines = [
        "Available Security Modes:",
        "  - always-ask   (Default: Ask for file writes and shell execution)",
        "  - accept-edits (Auto-allow file writes, ask for shell execution)",
        "  - plan-mode    (Read-only mode: File writes and shell execution are blocked)",
        "  - full-access  (YOLO mode: Auto-allow all tool calls without prompting)",
        "",
        `Current mode: ${current}`,
        "Use `/mode <mode_name>` to switch mode.",
      ];
      return { handled: true, message: lines.join("\n") };
    }

    if (!validModes.includes(targetMode)) {
      return {
        handled: true,
        message: `Invalid mode '${targetMode}'. Valid options: ${validModes.join(", ")}`,
      };
    }

    ctx.agent.setApprovalMode(targetMode as any);
    return {
      handled: true,
      message: `Switched security approval mode to: '${targetMode}'`,
    };
  },
};
