/**
 * Gemini CLI StreamFn — targets the Cloud Code Assist endpoint.
 *
 * Uses OAuth credentials from ~/.console/gemini-creds.json,
 * falls back to ~/.gemini/oauth_creds.json for compatibility.
 * No API key required.
 *
 * Endpoint: https://cloudcode-pa.googleapis.com
 * User-Agent: GeminiCLI/0.46.0/{modelId} (platform; arch; terminal)
 * Client-Metadata: ideType=IDE_UNSPECIFIED,...
 */
import type { StreamFn } from "../../../agent/src/service/agent-loop.js";
import { refreshIfNeeded } from "../auth/token-refresh.js";
import { loadCredential, credentialExists } from "../auth/token-store.js";
import { loginGemini } from "../auth/login.js";
import { buildEndpointUrl, convertMessages, convertTools, streamCore } from "../shared/index.js";
import type {
  CcaRequestPayload,
  CcaToolDeclarations,
  CloudCodeAssistRequest,
  GeminiFunctionDeclaration,
  GenerationConfig,
  SystemInstruction,
  SystemInstructionPart,
} from "../types/index.js";
import { GEMINI_BASE_URL, DEFAULT_GEMINI_CLI_VERSION } from "../constants.js";

function getGeminiCliVersion(): string {
  return process.env.GEMINI_CLI_VERSION ?? DEFAULT_GEMINI_CLI_VERSION;
}

function getGeminiUserAgent(modelId: string): string {
  const version = getGeminiCliVersion();
  const platform = process.platform === "win32" ? "win32" : process.platform;
  const arch = process.arch === "x64" ? "x64" : process.arch;
  return `GeminiCLI/${version}/${modelId} (${platform}; ${arch}; terminal)`;
}

function getGeminiHeaders(modelId: string): Record<string, string> {
  return {
    "User-Agent": getGeminiUserAgent(modelId),
    "Client-Metadata": "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI",
  };
}

function buildSystemInstruction(systemPrompt: string): SystemInstruction | undefined {
  if (!systemPrompt.trim()) return undefined;
  const part: SystemInstructionPart = { text: systemPrompt };
  const instruction: SystemInstruction = { role: undefined, parts: [part] };
  return instruction;
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

function buildGeminiRequest(
  projectId: string,
  modelId: string,
  systemPrompt: string,
  contents: ReturnType<typeof convertMessages>,
  tools: GeminiFunctionDeclaration[],
): CloudCodeAssistRequest {
  const generationConfig: GenerationConfig = {
    maxOutputTokens: 65536,
    temperature: undefined,
    thinkingConfig: undefined,
  };

  const payload: CcaRequestPayload = {
    contents,
    sessionId: undefined,
    systemInstruction: buildSystemInstruction(systemPrompt),
    generationConfig,
    tools: buildToolDeclarations(tools),
    toolConfig: undefined,
    labels: undefined,
  };

  const req: CloudCodeAssistRequest = {
    project: projectId,
    model: modelId,
    request: payload,
    requestId: undefined,
    requestType: undefined,
    userAgent: undefined,
  };

  return req;
}

export const geminiStreamFn: StreamFn = async function* ({
  model,
  systemPrompt,
  messages,
  tools,
  signal,
}) {
  // Check if credentials exist and are valid, auto-login if not
  let cred;
  try {
    const exists = await credentialExists("gemini");
    if (exists) {
      const rawCred = await loadCredential("gemini");
      cred = await refreshIfNeeded(rawCred, "gemini", signal);
    } else {
      throw new Error("No credentials");
    }
  } catch {
    // If any error (missing, invalid, missing projectId, etc.), re-login
    await loginGemini();
    const rawCred = await loadCredential("gemini");
    cred = await refreshIfNeeded(rawCred, "gemini", signal);
  }

  const baseUrl = (model as { baseUrl?: string }).baseUrl?.trim() ?? GEMINI_BASE_URL;
  const endpoint = buildEndpointUrl(baseUrl);

  const contents = convertMessages(messages);
  const functionDeclarations = convertTools(tools, model.id);
  const body = buildGeminiRequest(
    cred.projectId,
    model.id,
    systemPrompt,
    contents,
    functionDeclarations,
  );

  yield* streamCore({
    endpoint,
    accessToken: cred.accessToken,
    extraHeaders: getGeminiHeaders(model.id),
    body,
    signal,
  });
};
