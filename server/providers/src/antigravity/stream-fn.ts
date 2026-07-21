/**
 * Antigravity StreamFn — targets the daily-cloudcode-pa endpoint.
 *
 * Key differences from Gemini CLI:
 *  - User-Agent: antigravity/hub/{version} {os}/{arch}  (no Client-Metadata)
 *  - ANTIGRAVITY_SYSTEM_INSTRUCTION prepended to systemInstruction (role: "user")
 *  - toolConfig mode: VALIDATED
 *  - Request envelope: requestId, requestType="agent", labels, sessionId
 *  - Session state (agentId, trajectoryId, stepIndex) is stable per factory call
 *
 * Usage: call createAntigravityStreamFn() once per Agent instance.
 */
import type { StreamFn } from "../../../agent/src/service/agent-loop.js";
import { refreshIfNeeded } from "../auth/token-refresh.js";
import { loadCredential, credentialExists } from "../auth/token-store.js";
import { loginAntigravity } from "../auth/login.js";
import { buildEndpointUrl, convertMessages, convertTools, streamCore } from "../shared/index.js";
import type {
  CcaRequestPayload,
  CcaToolDeclarations,
  CloudCodeAssistRequest,
  FunctionCallingConfig,
  GeminiFunctionDeclaration,
  GenerationConfig,
  SystemInstruction,
  SystemInstructionPart,
  ToolConfig,
} from "../types/index.js";
import {
  buildEnvelope,
  createSessionState,
  type AntigravitySessionState,
} from "./session-envelope.js";

// ─────────────────────────────────────────────────────────────────────────────
// Constants
// ─────────────────────────────────────────────────────────────────────────────

import { ANTIGRAVITY_BASE_URL, DEFAULT_ANTIGRAVITY_VERSION } from "../constants.js";

/**
 * System instruction injected by the Antigravity client for Gemini 3 + Claude models.
 * Source: oh-my-pi/packages/catalog/src/wire/gemini-headers.ts
 */
const ANTIGRAVITY_SYSTEM_INSTRUCTION =
  "You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding." +
  "You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question." +
  "**Absolute paths only**" +
  "**Proactiveness**";

/** Max output tokens by wire model id (from ANTIGRAVITY_MODEL_WIRE_PROFILES) */
const MODEL_MAX_OUTPUT_TOKENS: Record<string, number> = {
  "gemini-3.5-flash-extra-low": 65536,
  "gemini-3.5-flash-low": 65536,
  "gemini-3-flash-agent": 65536,
  "gemini-3.1-pro-low": 65535,
  "gemini-pro-agent": 65535,
  "claude-sonnet-4-6": 64000,
  "claude-opus-4-6-thinking": 64000,
};

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function getAntigravityUserAgent(): string {
  const version = process.env.ANTIGRAVITY_VERSION ?? DEFAULT_ANTIGRAVITY_VERSION;
  const os = process.platform === "win32" ? "windows" : process.platform;
  const arch =
    process.arch === "x64" ? "amd64" : process.arch === "ia32" ? "386" : process.arch;
  return `antigravity/hub/${version} ${os}/${arch}`;
}

function shouldInjectSystemInstruction(modelId: string): boolean {
  const lower = modelId.toLowerCase();
  return lower.includes("claude") || lower.includes("gemini-3");
}

function isClaudeModel(modelId: string): boolean {
  return modelId.toLowerCase().includes("claude");
}

function buildSystemInstruction(modelId: string, userPrompt: string): SystemInstruction {
  const parts: SystemInstructionPart[] = [];

  if (shouldInjectSystemInstruction(modelId)) {
    parts.push({ text: ANTIGRAVITY_SYSTEM_INSTRUCTION });
  }
  if (userPrompt.trim()) {
    parts.push({ text: userPrompt });
  }

  const instruction: SystemInstruction = { role: "user", parts };
  return instruction;
}

function buildToolConfig(tools: GeminiFunctionDeclaration[], modelId: string): ToolConfig {
  const callingConfig: FunctionCallingConfig = {
    mode: "VALIDATED",
    allowedFunctionNames: undefined,
  };
  // Claude always forces VALIDATED even with no tools
  // Gemini with tools also uses VALIDATED per Antigravity behaviour
  void tools;
  void modelId;
  return { functionCallingConfig: callingConfig };
}

function buildToolDeclarations(
  tools: GeminiFunctionDeclaration[],
): CcaToolDeclarations[] | undefined {
  if (tools.length === 0) return undefined;
  const declarations: CcaToolDeclarations = {
    functionDeclarations: tools as unknown as Record<string, unknown>[],
  };
  return [declarations];
}

function buildAntigravityRequest(
  projectId: string,
  modelId: string,
  systemPrompt: string,
  contents: ReturnType<typeof convertMessages>,
  tools: GeminiFunctionDeclaration[],
  sessionState: AntigravitySessionState,
): CloudCodeAssistRequest {
  const envelope = buildEnvelope(sessionState, modelId);
  const maxOutputTokens = MODEL_MAX_OUTPUT_TOKENS[modelId] ?? 65536;

  const generationConfig: GenerationConfig = {
    maxOutputTokens,
    temperature: undefined,
    thinkingConfig: undefined,
  };

  const payload: CcaRequestPayload = {
    contents,
    sessionId: envelope.sessionId,
    systemInstruction: buildSystemInstruction(modelId, systemPrompt),
    generationConfig,
    tools: buildToolDeclarations(tools),
    toolConfig: (tools.length > 0 || isClaudeModel(modelId))
      ? buildToolConfig(tools, modelId)
      : undefined,
    labels: envelope.labels,
  };

  const req: CloudCodeAssistRequest = {
    project: projectId,
    model: modelId,
    request: payload,
    requestId: envelope.requestId,
    requestType: "agent",
    userAgent: "antigravity",
  };

  return req;
}

// ─────────────────────────────────────────────────────────────────────────────
// Factory
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Creates an Antigravity StreamFn with its own stable session state.
 * Call once per Agent instance — the returned function keeps stepIndex
 * and sessionId consistent across multiple run() calls.
 */
export function createAntigravityStreamFn(): StreamFn {
  const sessionState: AntigravitySessionState = createSessionState();

  return async function* ({ model, systemPrompt, messages, tools, signal }) {
    // Check if credentials exist and are valid, auto-login if not
    let cred;
    try {
      const exists = await credentialExists("antigravity");
      if (exists) {
        const rawCred = await loadCredential("antigravity");
        cred = await refreshIfNeeded(rawCred, "antigravity", signal);
      } else {
        throw new Error("No credentials");
      }
    } catch {
      // If any error (missing, invalid, missing projectId, etc.), re-login
      await loginAntigravity();
      const rawCred = await loadCredential("antigravity");
      cred = await refreshIfNeeded(rawCred, "antigravity", signal);
    }

    const baseUrl = (model as { baseUrl?: string }).baseUrl?.trim() ?? ANTIGRAVITY_BASE_URL;
    const endpoint = buildEndpointUrl(baseUrl);

    const contents = convertMessages(messages);
    const functionDeclarations = convertTools(tools);
    const body = buildAntigravityRequest(
      cred.projectId,
      model.id,
      systemPrompt,
      contents,
      functionDeclarations,
      sessionState,
    );

    yield* streamCore({
      endpoint,
      accessToken: cred.accessToken,
      extraHeaders: { "User-Agent": getAntigravityUserAgent() },
      body,
      signal,
    });
  };
}
