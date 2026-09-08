/**
 * Devin provider tests.
 * Covers OAuth (Phase 2), Connect streaming + framing (Phase 3), message
 * transformation, and model discovery. All tests are offline — fetch is
 * mocked. The live test path (DEVIN_REAL_API=1) is left for a follow-up
 * since it needs a real Devin account.
 */
import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { gzipSync, gunzipSync } from "node:zlib";
import {
  ChatMessagePromptSchema,
  ChatMessageSource,
  ChatToolCallSchema,
  ChatMessageRequestType,
  ChatToolChoiceSchema,
  CompletionConfigurationSchema,
  ConversationalPlannerMode,
  CacheControlType,
  ChatToolDefinitionSchema,
  GetChatMessageResponseSchema,
  GetCliModelConfigsResponseSchema,
  GetUserJwtResponseSchema,
  MetadataSchema,
  PromptCacheOptionsSchema,
} from "@/providers/src/devin/proto/devin-proto.js";
import { create, fromBinary, toBinary } from "@/providers/src/devin/proto/protobuf.js";
import {
  createDevinAuthorizationUrl,
  decodeJwtPayload,
  devinCredentialExists,
  exchangeDevinCode,
  fetchDevinModels,
  generateDevinPkce,
  getTokenExpiry,
  loadDevinCredential,
  parseDevinCredential,
  refreshDevinIfNeeded,
  saveDevinCredential,
} from "@/providers/src/devin/index.js";
import { devinStreamFn } from "@/providers/src/devin/stream-fn.js";
import { buildChatMessagePrompts, buildDevinTools } from "@/providers/src/devin/transform-messages.js";

console.log("Running Devin provider tests...");

// ─── OAuth (Phase 2) ───────────────────────────────────────────────────────

// 1. PKCE verifier/challenge are well-formed
{
  const { verifier, challenge } = generateDevinPkce();
  assert.ok(verifier.length >= 32, "verifier must be long enough");
  assert.ok(/^[A-Za-z0-9_-]+$/.test(verifier), "verifier must be base64url");
  assert.ok(/^[A-Za-z0-9_-]+$/.test(challenge), "challenge must be base64url");
  assert.notEqual(verifier, challenge, "verifier and challenge must differ");
  console.log("  ✅ PKCE verifier/challenge well-formed");
}

// 2. Authorization URL contains expected params + correct redirect
{
  const { authUrl, redirectUri } = createDevinAuthorizationUrl({
    state: "abc123",
    verifierChallenge: "challenge-xyz",
  });
  assert.ok(authUrl.startsWith("https://app.devin.ai/auth/cli/continue?"));
  const params = new URL(authUrl).searchParams;
  assert.equal(params.get("state"), "abc123");
  assert.equal(params.get("code_challenge"), "challenge-xyz");
  assert.equal(params.get("code_challenge_method"), "S256");
  assert.equal(params.get("prompt"), "select_account");
  assert.equal(params.get("redirect_uri"), redirectUri);
  assert.equal(redirectUri, "http://127.0.0.1:59653/callback");
  console.log("  ✅ authorization URL has expected PKCE params");
}

// 3. JWT expiry parsing — uses a synthetic JWT (header.payload.sig)
{
  const payload = Buffer.from(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600 }))
    .toString("base64url");
  const token = `aaa.${payload}.bbb`;
  const expiry = getTokenExpiry(token);
  const delta = expiry - Date.now();
  assert.ok(delta > 50 * 60 * 1000, `expected ~55m expiry, got ${delta}ms`);
  assert.ok(delta < 60 * 60 * 1000, `expected ~55m expiry, got ${delta}ms`);
  console.log("  ✅ JWT expiry parsed correctly with skew");
}

// 4. Non-JWT tokens fall back to a long-lived estimate
{
  const expiry = getTokenExpiry("not-a-jwt");
  assert.ok(expiry > Date.now() + 300 * 24 * 60 * 60 * 1000, "fallback should be > 300 days");
  console.log("  ✅ non-JWT falls back to long-lived estimate");
}

// 5. decodeJwtPayload tolerates garbage input
{
  assert.equal(decodeJwtPayload(""), null);
  assert.equal(decodeJwtPayload("only.two"), null);
  assert.equal(decodeJwtPayload("a.b.c"), null);
  assert.equal(decodeJwtPayload("a..c"), null);
  const valid = `aaa.${Buffer.from(JSON.stringify({ x: 1 })).toString("base64url")}.ccc`;
  assert.deepEqual(decodeJwtPayload(valid), { x: 1 });
  console.log("  ✅ decodeJwtPayload handles bad input safely");
}

// 6. Credential round-trip via filesystem (env-driven path)
{
  const dir = mkdtempSync(join(tmpdir(), "devin-creds-"));
  process.env.DEVIN_CREDENTIALS_PATH = join(dir, "devin-creds.json");
  try {
    assert.equal(await devinCredentialExists(), false);

    const payload = Buffer.from(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 7200 }))
      .toString("base64url");
    const token = `aaa.${payload}.bbb`;
    await saveDevinCredential({
      access_token: token,
      refresh_token: token,
      expiresAt: getTokenExpiry(token),
    });

    assert.equal(await devinCredentialExists(), true);
    const loaded = await loadDevinCredential();
    assert.equal(loaded.accessToken, token);
    assert.equal(loaded.refreshToken, token);
    console.log("  ✅ save → exists → load round-trip");
  } finally {
    rmSync(dir, { recursive: true, force: true });
    delete process.env.DEVIN_CREDENTIALS_PATH;
  }
}

// 7. parseDevinCredential rejects empty token
{
  assert.throws(() => parseDevinCredential({ access_token: "", refresh_token: "", expiresAt: 0 }), /Invalid Devin/);
  console.log("  ✅ parseDevinCredential rejects empty token");
}

// 8. refreshDevinIfNeeded picks up rotated credentials on disk when expired
{
  const dir = mkdtempSync(join(tmpdir(), "devin-creds-"));
  process.env.DEVIN_CREDENTIALS_PATH = join(dir, "devin-creds.json");
  try {
    const expired = {
      access_token: "old-token",
      refresh_token: "old-token",
      expiresAt: Date.now() - 60_000,
    };
    await saveDevinCredential(expired);
    await saveDevinCredential({
      access_token: "new-token",
      refresh_token: "new-token",
      expiresAt: Date.now() + 3600_000,
    });
    const refreshed = await refreshDevinIfNeeded({
      accessToken: "old-token",
      refreshToken: "old-token",
      expiresAtMs: expired.expiresAt,
    });
    assert.equal(refreshed.accessToken, "new-token");
    console.log("  ✅ refreshDevinIfNeeded loads rotated credential from disk");
  } finally {
    rmSync(dir, { recursive: true, force: true });
    delete process.env.DEVIN_CREDENTIALS_PATH;
  }
}

// 9. exchangeDevinCode against a mocked /auth/cli/token endpoint
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ token: "cli-session-token" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })) as unknown as typeof fetch;
  try {
    const cred = await exchangeDevinCode("auth-code", "verifier", "http://127.0.0.1:59653/callback");
    assert.equal(cred.access_token, "cli-session-token");
    assert.equal(cred.refresh_token, "cli-session-token");
    console.log("  ✅ exchangeDevinCode parses { token } response");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 10. exchangeDevinCode surfaces non-2xx with status code
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response("invalid_grant", {
      status: 400,
      headers: { "Content-Type": "text/plain" },
    })) as unknown as typeof fetch;
  try {
    await assert.rejects(
      () => exchangeDevinCode("bad", "verifier", "http://127.0.0.1:59653/callback"),
      /Devin OAuth token exchange failed \(400\)/,
    );
    console.log("  ✅ exchangeDevinCode surfaces non-2xx");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 11. exchangeDevinCode rejects empty token in successful response
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ token: "" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })) as unknown as typeof fetch;
  try {
    await assert.rejects(() => exchangeDevinCode("code", "v", "http://127.0.0.1:59653/callback"), /empty token/);
    console.log("  ✅ exchangeDevinCode rejects empty token");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// ─── Streaming protocol (Phase 3) ─────────────────────────────────────────

/** Build a minimal valid GetChatMessageResponse for testing. */
function makeResponse(opts: {
  deltaText?: string;
  deltaThinking?: string;
  deltaToolCalls?: Array<{ id?: string; name?: string; argumentsJson?: string }>;
}): Uint8Array {
  return toBinary(
    GetChatMessageResponseSchema,
    create(GetChatMessageResponseSchema, {
      deltaText: opts.deltaText ?? "",
      deltaThinking: opts.deltaThinking ?? "",
      deltaToolCalls: (opts.deltaToolCalls ?? []).map((tc) =>
        create(ChatToolCallSchema, {
          id: tc.id ?? "",
          name: tc.name ?? "",
          argumentsJson: tc.argumentsJson ?? "",
        }),
      ),
    }),
  );
}

/** Wrap a payload in a single Connect frame: 1 flag byte + 4 BE length + payload. */
function frame(payload: Uint8Array, gzip = true): Uint8Array {
  const body = gzip ? gzipSync(payload) : payload;
  const out = new Uint8Array(5 + body.length);
  out[0] = gzip ? 0x01 : 0x00;
  new DataView(out.buffer).setUint32(1, body.length, false);
  out.set(body, 5);
  return out;
}

/** Stitch an arbitrary list of frame bodies into a fake stream. */
function streamFrom(...bodies: Uint8Array[]): ReadableStream<Uint8Array> {
  return new ReadableStream({
    start(controller) {
      for (const body of bodies) {
        controller.enqueue(frame(body) as unknown as Uint8Array);
      }
      controller.close();
    },
  });
}

/**
 * Build a fake `fetch` that:
 *   - returns a valid GetUserJwt response (with `userJwt: "test-jwt"`) for auth calls
 *   - delegates chat calls to the supplied `chatResponder`
 *
 * devinStreamFn calls GetUserJwt before each stream; tests override only the
 * chat side and reuse this for auth.
 */
function fetchMockWithAuth(chatResponder: (url: string) => Response | Promise<Response>): typeof fetch {
  const authPayload = toBinary(
    GetUserJwtResponseSchema,
    create(GetUserJwtResponseSchema, { userJwt: "test-jwt" }),
  );
  const authResponse = new Response(authPayload as unknown as BodyInit, {
    status: 200,
    headers: { "Content-Type": "application/proto" },
  });
  return (async (url: string | URL) => {
    const u = typeof url === "string" ? url : url.toString();
    if (u.includes("/GetUserJwt")) return authResponse;
    return chatResponder(u);
  }) as unknown as typeof fetch;
}

// 12. Connect frame round-trip — encoding/decoding gzip payloads
{
  const payload = makeResponse({ deltaText: "hello" });
  const gz = gzipSync(payload);
  assert.equal(gunzipSync(gz).byteLength, payload.byteLength);
  const back = fromBinary(GetChatMessageResponseSchema, gunzipSync(gz));
  assert.equal(back.deltaText, "hello");
  console.log("  ✅ gzip frame round-trip preserves payload");
}

// 13. devinStreamFn emits text deltas from a single-framed response
{
  process.env.DEVIN_OAUTH_TOKEN = "cli-session-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = fetchMockWithAuth(() =>
    new Response(streamFrom(makeResponse({ deltaText: "hello, world" })), {
      status: 200,
      headers: { "Content-Type": "application/connect+proto" },
    }),
  );
  try {
    const deltas: Array<{ type: string; text?: string }> = [];
    for await (const d of devinStreamFn({
      model: { id: "claude-3-7-sonnet", provider: "devin", contextWindow: 200_000 },
      systemPrompt: "hi",
      messages: [{ role: "user", content: "say hi" }],
      tools: [],
    })) {
      deltas.push(d as { type: string; text?: string });
    }
    const textDeltas = deltas.filter((d) => d.type === "text");
    assert.equal(textDeltas.length, 1);
    assert.equal(textDeltas[0]!.text, "hello, world");
    console.log("  ✅ devinStreamFn emits text deltas");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.DEVIN_OAUTH_TOKEN;
  }
}

// 14. devinStreamFn emits thinking deltas (consecutive frames concatenate)
{
  process.env.DEVIN_OAUTH_TOKEN = "cli-session-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = fetchMockWithAuth(() =>
    new Response(
      streamFrom(
        makeResponse({ deltaThinking: "reasoning step 1" }),
        makeResponse({ deltaThinking: " + step 2" }),
      ),
      { status: 200, headers: { "Content-Type": "application/connect+proto" } },
    ),
  );
  try {
    const thinkingTexts: string[] = [];
    for await (const d of devinStreamFn({
      model: { id: "claude-3-7-sonnet", provider: "devin", contextWindow: 200_000 },
      systemPrompt: "hi",
      messages: [{ role: "user", content: "think first" }],
      tools: [],
    })) {
      if (d.type === "thinking") thinkingTexts.push(d.text);
    }
    assert.equal(thinkingTexts.join(""), "reasoning step 1 + step 2");
    console.log("  ✅ devinStreamFn emits thinking deltas");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.DEVIN_OAUTH_TOKEN;
  }
}

// 15. devinStreamFn accumulates tool-call argument deltas (prefix semantics)
{
  process.env.DEVIN_OAUTH_TOKEN = "cli-session-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = fetchMockWithAuth(() =>
    new Response(
      streamFrom(
        makeResponse({ deltaToolCalls: [{ id: "tc-1", name: "bash", argumentsJson: '{"command":' }] }),
        makeResponse({ deltaToolCalls: [{ argumentsJson: '{"command":"ls' }] }),
        makeResponse({ deltaToolCalls: [{ argumentsJson: '{"command":"ls"}' }] }),
      ),
      { status: 200, headers: { "Content-Type": "application/connect+proto" } },
    ),
  );
  try {
    const toolDeltas: Array<{ id: string; name: string; argumentsJson: string }> = [];
    for await (const d of devinStreamFn({
      model: { id: "claude-3-7-sonnet", provider: "devin", contextWindow: 200_000 },
      systemPrompt: "hi",
      messages: [{ role: "user", content: "run ls" }],
      tools: [],
    })) {
      if (d.type === "toolCall") toolDeltas.push(d);
    }
    assert.equal(toolDeltas[0]!.id, "tc-1");
    assert.equal(toolDeltas[0]!.name, "bash");
    // All emitted args deltas, concatenated, must reconstruct the full JSON.
    const assembled = toolDeltas.map((d) => d.argumentsJson).join("");
    assert.deepEqual(JSON.parse(assembled), { command: "ls" });
    console.log("  ✅ devinStreamFn accumulates tool-call args as incremental deltas");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.DEVIN_OAUTH_TOKEN;
  }
}

// 16. devinStreamFn surfaces end-of-stream trailer errors
{
  process.env.DEVIN_OAUTH_TOKEN = "cli-session-token";
  const originalFetch = globalThis.fetch;
  const trailerJson = JSON.stringify({ error: { code: "invalid_argument", message: "internal error" } });
  const trailerFrame = new Uint8Array(5 + trailerJson.length);
  trailerFrame[0] = 0x02;
  new DataView(trailerFrame.buffer).setUint32(1, trailerJson.length, false);
  new TextEncoder().encodeInto(trailerJson, trailerFrame.subarray(5));
  globalThis.fetch = fetchMockWithAuth(() =>
    new Response(
      new ReadableStream({
        start(controller) {
          controller.enqueue(frame(makeResponse({ deltaText: "partial" })));
          controller.enqueue(trailerFrame);
          controller.close();
        },
      }),
      { status: 200, headers: { "Content-Type": "application/connect+proto" } },
    ),
  );
  try {
    const collected: string[] = [];
    await assert.rejects(
      (async () => {
        for await (const d of devinStreamFn({
          model: { id: "claude-3-7-sonnet", provider: "devin", contextWindow: 200_000 },
          systemPrompt: "hi",
          messages: [{ role: "user", content: "say hi" }],
          tools: [],
        })) {
          if (d.type === "text") collected.push(d.text);
        }
      })(),
      /invalid_argument: internal error/,
    );
    assert.deepEqual(collected, ["partial"]);
    console.log("  ✅ devinStreamFn surfaces trailer errors");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.DEVIN_OAUTH_TOKEN;
  }
}

// 17. devinStreamFn rejects oversize Connect frame length prefix
{
  process.env.DEVIN_OAUTH_TOKEN = "cli-session-token";
  const originalFetch = globalThis.fetch;
  const oversized = new Uint8Array(5);
  oversized[0] = 0x00;
  new DataView(oversized.buffer).setUint32(1, 0xffffffff, false);
  globalThis.fetch = fetchMockWithAuth(() =>
    new Response(
      new ReadableStream({
        start(controller) {
          controller.enqueue(oversized);
          controller.close();
        },
      }),
      { status: 200, headers: { "Content-Type": "application/connect+proto" } },
    ),
  );
  try {
    await assert.rejects(
      (async () => {
        for await (const _ of devinStreamFn({
          model: { id: "claude-3-7-sonnet", provider: "devin", contextWindow: 200_000 },
          systemPrompt: "hi",
          messages: [{ role: "user", content: "x" }],
          tools: [],
        })) {
          // drain
        }
      })(),
      /exceeds 16777216-byte cap/,
    );
    console.log("  ✅ devinStreamFn caps Connect frame length");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.DEVIN_OAUTH_TOKEN;
  }
}

// ─── Message transformation (transform-messages) ──────────────────────────

// 18. buildChatMessagePrompts maps user/assistant/toolResult roles
{
  const out = buildChatMessagePrompts(
    [
      { role: "user", content: "hi", id: "u1" },
      {
        role: "assistant",
        id: "a1",
        content: [
          { type: "text", text: "hello" },
          { type: "thinking", text: "first" },
        ],
      },
      {
        role: "toolResult",
        id: "t1",
        results: [{ toolCallId: "tc-1", content: { ok: true } }],
      },
    ],
    "cascade-1",
  );
  assert.equal(out.length, 3);
  assert.equal(out[0]!.source, ChatMessageSource.USER);
  assert.equal(out[0]!.prompt, "hi");
  assert.equal(out[1]!.source, ChatMessageSource.SYSTEM);
  assert.equal(out[1]!.prompt, "hello");
  assert.equal(out[1]!.thinking, "first");
  assert.equal(out[2]!.source, ChatMessageSource.TOOL);
  assert.equal(out[2]!.toolCallId, "tc-1");
  console.log("  ✅ buildChatMessagePrompts maps Console → Cascade roles");
}

// 19. buildDevinTools produces JSON-schema strings
{
  const { z } = await import("zod");
  const tools = buildDevinTools([
    {
      name: "bash",
      description: "run a command",
      inputSchema: z.object({ command: z.string() }),
      execute: async () => ({}),
    },
  ]);
  assert.equal(tools[0]!.name, "bash");
  assert.equal(tools[0]!.strict, false);
  const schema = JSON.parse(tools[0]!.jsonSchemaString);
  assert.equal(schema.type, "object");
  assert.ok(schema.properties.command);
  assert.equal(schema.required[0], "command");
  console.log("  ✅ buildDevinTools serialises tools to JSON schema");
}

// ─── Model discovery (fetchDevinModels) ─────────────────────────────────────

// 20. fetchDevinModels decodes a unary response into enabled models
{
  process.env.DEVIN_OAUTH_TOKEN = "cli-session-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () => {
    const payload = toBinary(
      GetCliModelConfigsResponseSchema,
      create(GetCliModelConfigsResponseSchema, {
        clientModelConfigs: [
          {
            modelUid: "claude-3-7-sonnet",
            label: "Claude 3.7 Sonnet",
            disabled: false,
            maxTokens: 200_000,
            supportsImages: true,
          } as never,
          { modelUid: "hidden", label: "Hidden", disabled: true } as never,
          { modelUid: "", label: "No UID", disabled: false } as never,
        ],
      }),
    );
    return new Response(payload as unknown as BodyInit, { status: 200, headers: { "Content-Type": "application/proto" } });
  }) as unknown as typeof fetch;
  try {
    const models = await fetchDevinModels();
    assert.ok(models);
    assert.equal(models!.length, 1);
    assert.equal(models![0]!.id, "claude-3-7-sonnet");
    assert.equal(models![0]!.name, "Claude 3.7 Sonnet");
    assert.equal(models![0]!.contextWindow, 200_000);
    assert.equal(models![0]!.supportsImages, true);
    console.log("  ✅ fetchDevinModels decodes enabled configs");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.DEVIN_OAUTH_TOKEN;
  }
}

// 21. fetchDevinModels returns null on non-2xx
{
  process.env.DEVIN_OAUTH_TOKEN = "cli-session-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () => new Response("nope", { status: 500 })) as unknown as typeof fetch;
  try {
    const models = await fetchDevinModels();
    assert.equal(models, null);
    console.log("  ✅ fetchDevinModels surfaces auth/network failures");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.DEVIN_OAUTH_TOKEN;
  }
}

// 22. Provider catalog advertises Devin
{
  const { PROVIDER_CATALOG, listProviders } = await import("@/agent/src/commands/provider-registry.js");
  const devinEntry = PROVIDER_CATALOG.devin;
  assert.ok(devinEntry, "devin should be in PROVIDER_CATALOG");
  assert.equal(devinEntry!.name, "devin");
  assert.equal(devinEntry!.authMethod, "oauth");
  assert.ok(typeof devinEntry!.getStreamFn === "function");
  assert.ok(devinEntry!.models.length > 0);

  const listed = listProviders().find((p) => p.name === "devin");
  assert.ok(listed, "devin should be returned by listProviders()");
  console.log("  ✅ PROVIDER_CATALOG and listProviders include devin");
}

console.log("Devin provider tests passed!\n");
process.exit(0);
