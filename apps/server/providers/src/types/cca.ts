/**
 * Cloud Code Assist (CCA) wire types.
 *
 * Split into two groups:
 *  - Outgoing types: what we BUILD and POST to the endpoint
 *  - Incoming types: what we RECEIVE from the SSE stream
 *
 * All types use explicit named interfaces — no inline object literals.
 */

// ─────────────────────────────────────────────────────────────────────────────
// Outgoing — content parts we send
// ─────────────────────────────────────────────────────────────────────────────

/** A plain text part in a Gemini Content turn */
export interface GeminiTextPart {
  text: string;
  thoughtSignature?: string;
}

/** Inline (base64) media part for multimodal input. */
export interface GeminiInlineDataPart {
  inlineData: {
    mimeType: string;
    data: string;
  };
}

/** A function call reference stored inside a model turn */
export interface GeminiFunctionCallRef {
  name: string;
  args: Record<string, unknown>;
  id: string;
}

/** A function call part in a model (assistant) turn */
export interface GeminiFunctionCallPart {
  functionCall: GeminiFunctionCallRef;
  thoughtSignature?: string;
}

/** The response payload nested inside a functionResponse part */
export interface GeminiFunctionResponseBody {
  content: unknown;
}

/** The function response reference stored inside a user turn */
export interface GeminiFunctionResponseRef {
  name: string;
  id: string;
  response: GeminiFunctionResponseBody;
}

/** A function response part in a user (tool-result) turn */
export interface GeminiFunctionResponsePart {
  functionResponse: GeminiFunctionResponseRef;
}

/** Discriminated union of parts we produce for outgoing Content turns */
export type GeminiOutgoingPart =
  | GeminiTextPart
  | GeminiInlineDataPart
  | GeminiFunctionCallPart
  | GeminiFunctionResponsePart;

/** A single Gemini conversation turn */
export interface GeminiContent {
  role: "user" | "model";
  parts: GeminiOutgoingPart[];
}

/** A single function declaration as sent in the tools array */
export interface GeminiFunctionDeclaration {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
}

// ─────────────────────────────────────────────────────────────────────────────
// Incoming — content parts we receive from the SSE stream
// ─────────────────────────────────────────────────────────────────────────────

/**
 * A function call received from the model.
 * `id` is optional because some Gemini versions omit it.
 */
export interface IncomingFunctionCall {
  name: string;
  args: Record<string, unknown>;
  id: string | undefined;
}

/**
 * A raw CCA response part. The Gemini wire format uses optional overlapping
 * fields rather than a discriminant, so we model it with optionals here and
 * narrow with type guards in stream-core.ts.
 */
export interface CcaResponsePart {
  text: string | undefined;
  /** True when this part contains model reasoning (thinking), not visible output */
  thought: boolean | undefined;
  thoughtSignature: string | undefined;
  functionCall: IncomingFunctionCall | undefined;
}

// ─────────────────────────────────────────────────────────────────────────────
// Request shape
// ─────────────────────────────────────────────────────────────────────────────

/** Thinking configuration for Gemini models */
export interface ThinkingConfig {
  includeThoughts: boolean;
  thinkingBudget: number | undefined;
  thinkingLevel: string | undefined;
}

/** Tool function calling mode */
export type FunctionCallingMode = "AUTO" | "NONE" | "ANY" | "VALIDATED";

/** Tool calling configuration */
export interface FunctionCallingConfig {
  mode: FunctionCallingMode;
  allowedFunctionNames: string[] | undefined;
}

/** Wraps the tool calling config */
export interface ToolConfig {
  functionCallingConfig: FunctionCallingConfig;
}

/** Generation parameters */
export interface GenerationConfig {
  maxOutputTokens: number | undefined;
  temperature: number | undefined;
  thinkingConfig: ThinkingConfig | undefined;
}

/** System instruction block */
export interface SystemInstruction {
  role: string | undefined;
  parts: SystemInstructionPart[];
}

/** A single text part within a system instruction */
export interface SystemInstructionPart {
  text: string;
}

/** The tool declarations wrapper */
export interface CcaToolDeclarations {
  functionDeclarations: Record<string, unknown>[];
}

/** The inner request payload */
export interface CcaRequestPayload {
  contents: GeminiContent[];
  sessionId: string | undefined;
  systemInstruction: SystemInstruction | undefined;
  generationConfig: GenerationConfig | undefined;
  tools: CcaToolDeclarations[] | undefined;
  toolConfig: ToolConfig | undefined;
  labels: Record<string, string> | undefined;
}

/** The full Cloud Code Assist request envelope */
export interface CloudCodeAssistRequest {
  project: string;
  model: string;
  request: CcaRequestPayload;
  requestId: string | undefined;
  requestType: string | undefined;
  userAgent: string | undefined;
}

// ─────────────────────────────────────────────────────────────────────────────
// Response shape (SSE chunks)
// ─────────────────────────────────────────────────────────────────────────────

/** Token usage metadata from the response */
export interface CcaUsageMetadata {
  promptTokenCount: number | undefined;
  candidatesTokenCount: number | undefined;
  totalTokenCount: number | undefined;
}

/** The content block within a response candidate */
export interface CcaResponseContent {
  role: string;
  parts: CcaResponsePart[];
}

/** A single response candidate */
export interface CcaResponseCandidate {
  content: CcaResponseContent | undefined;
  finishReason: string | undefined;
}

/** The response object within a CCA SSE chunk */
export interface CcaResponse {
  candidates: CcaResponseCandidate[] | undefined;
  usageMetadata: CcaUsageMetadata | undefined;
  responseId: string | undefined;
}

/** An in-band stream error delivered as a final SSE event */
export interface CcaStreamError {
  code: number | undefined;
  message: string | undefined;
  status: string | undefined;
}

/** A single parsed SSE event from the CCA stream */
export interface CloudCodeAssistChunk {
  response: CcaResponse | undefined;
  error: CcaStreamError | undefined;
  traceId: string | undefined;
}
