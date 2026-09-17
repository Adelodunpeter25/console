/**
 * Claude provider tests.
 * Covers OAuth (PKCE URL, token exchange, refresh, credential storage),
 * Anthropic wire converters, SSE parsing, usage normalization, and the
 * streaming tool-call flow. All tests are offline — fetch is mocked.
 */
import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { z } from "zod";
import {
  CLAUDE_CALLBACK_PATH,
  CLAUDE_CALLBACK_PORT,
  CLAUDE_CLIENT_ID,
  claudeCredentialExists,
  claudeRedirectUri,
  convertClaudeMessages,
  convertClaudeTools,
  createClaudeAuthorizationUrl,
  exchangeClaudeCode,
  generateClaudePkce,
  loadClaudeCredential,
  normalizeClaudeUsage,
  parseAnthropicSse,
  parseClaudeCredential,
  refreshClaudeIfNeeded,
  saveClaudeCredential,
} from "@/providers/src/claude/index.js";
import { claudeStreamFn } from "@/providers/src/claude/stream-fn.js";
import { fetchClaudeModels } from "@/providers/src/claude/discovery.js";

console.log("Running Claude provider tests...");

// ─── OAuth ─────────────────────────────────────────────────────────────────

// 1. PKCE verifier/challenge are well-formed
{
  const { verifier, challenge } = generateClaudePkce();
  assert.ok(verifier.length >= 32, "verifier must be long enough");
  assert.ok(/^[A-Za-z0-9_-]+$/.test(verifier), "verifier must be base64url");
  assert.ok(/^[A-Za-z0-9_-]+$/.test(challenge), "challenge must be base64url");
  assert.notEqual(verifier, challenge, "verifier and challenge must differ");
  console.log("  ✅ PKCE verifier/challenge well-formed");
}

// 2. Authorization URL contains expected params + correct redirect
{
  const { authUrl, redirectUri } = createClaudeAuthorizationUrl({
    state: "state-123",
    verifierChallenge: "challenge-xyz",
  });
  assert.ok(authUrl.startsWith("https://claude.ai/oauth/authorize?"));
  const params = new URL(authUrl).searchParams;
  assert.equal(params.get("client_id"), CLAUDE_CLIENT_ID);
  assert.match(
    CLAUDE_CLIENT_ID,
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    "client_id must be a valid UUID",
  );
  assert.equal(params.get("response_type"), "code");
  assert.equal(params.get("code_challenge"), "challenge-xyz");
  assert.equal(params.get("code_challenge_method"), "S256");
  assert.equal(params.get("state"), "state-123");
  assert.equal(params.get("code"), "true");
  assert.ok((params.get("scope") ?? "").includes("user:inference"));
  assert.equal(params.get("redirect_uri"), redirectUri);
  assert.equal(redirectUri, `http://localhost:${CLAUDE_CALLBACK_PORT}${CLAUDE_CALLBACK_PATH}`);
  assert.equal(claudeRedirectUri(), redirectUri);
  console.log("  ✅ authorization URL has expected PKCE params");
}

// 3. parseClaudeCredential rejects empty tokens
{
  assert.throws(
    () => parseClaudeCredential({ access_token: "", refresh_token: "", expiresAt: 0 }),
    /Invalid Claude/,
  );
  console.log("  ✅ parseClaudeCredential rejects empty token");
}

// 4. Credential round-trip via filesystem (env-driven path)
{
  const dir = mkdtempSync(join(tmpdir(), "claude-creds-"));
  process.env.CLAUDE_CREDENTIALS_PATH = join(dir, "claude-creds.json");
  try {
    assert.equal(await claudeCredentialExists(), false);
    await saveClaudeCredential({
      access_token: "access-1",
      refresh_token: "refresh-1",
      expiresAt: Date.now() + 3600_000,
      email: "user@example.com",
    });
    assert.equal(await claudeCredentialExists(), true);
    const loaded = await loadClaudeCredential();
    assert.equal(loaded.accessToken, "access-1");
    assert.equal(loaded.refreshToken, "refresh-1");
    assert.equal(loaded.email, "user@example.com");
    console.log("  ✅ save → exists → load round-trip");
  } finally {
    rmSync(dir, { recursive: true, force: true });
    delete process.env.CLAUDE_CREDENTIALS_PATH;
  }
}

// 5. exchangeClaudeCode parses the JSON token response
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async (url: string | URL | Request) => {
    const u = typeof url === "string" ? url : url.toString();
    if (u.includes("/oauth/token")) {
      return new Response(
        JSON.stringify({ access_token: "access-tok", refresh_token: "refresh-tok", expires_in: 3600 }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    // Bootstrap identity lookup — no account email in this fixture.
    return new Response(JSON.stringify({}), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as unknown as typeof fetch;
  try {
    const cred = await exchangeClaudeCode("code-1", "state-1", "verifier-1", claudeRedirectUri());
    assert.equal(cred.access_token, "access-tok");
    assert.equal(cred.refresh_token, "refresh-tok");
    assert.ok(cred.expiresAt > Date.now());
    assert.equal(cred.email, undefined);
    console.log("  ✅ exchangeClaudeCode parses JSON token response");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 6. exchangeClaudeCode surfaces non-2xx with status code
{
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response("invalid_grant", { status: 400 })) as unknown as typeof fetch;
  try {
    await assert.rejects(
      () => exchangeClaudeCode("bad", "state", "verifier", claudeRedirectUri()),
      /Claude OAuth request failed \(400\)/,
    );
    console.log("  ✅ exchangeClaudeCode surfaces non-2xx");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 7. refreshClaudeIfNeeded rotates tokens near expiry and persists them
{
  const dir = mkdtempSync(join(tmpdir(), "claude-creds-"));
  process.env.CLAUDE_CREDENTIALS_PATH = join(dir, "claude-creds.json");
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async (url: string | URL | Request) => {
    const u = typeof url === "string" ? url : url.toString();
    if (u.includes("/oauth/token")) {
      return new Response(
        JSON.stringify({ access_token: "new-access", refresh_token: "new-refresh", expires_in: 3600 }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    return new Response(JSON.stringify({}), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as unknown as typeof fetch;
  try {
    const refreshed = await refreshClaudeIfNeeded({
      accessToken: "old-access",
      refreshToken: "old-refresh",
      expiresAtMs: Date.now() - 60_000,
    });
    assert.equal(refreshed.accessToken, "new-access");
    assert.equal(refreshed.refreshToken, "new-refresh");
    const onDisk = await loadClaudeCredential();
    assert.equal(onDisk.accessToken, "new-access");
    console.log("  ✅ refreshClaudeIfNeeded rotates + persists near expiry");
  } finally {
    globalThis.fetch = originalFetch;
    rmSync(dir, { recursive: true, force: true });
    delete process.env.CLAUDE_CREDENTIALS_PATH;
  }
}

// 8. refreshClaudeIfNeeded leaves fresh credentials untouched (no network)
{
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = (async () => {
    calls++;
    return new Response("{}", { status: 200 });
  }) as unknown as typeof fetch;
  try {
    const cred = {
      accessToken: "fresh",
      refreshToken: "refresh",
      expiresAtMs: Date.now() + 3600_000,
    };
    assert.deepEqual(await refreshClaudeIfNeeded(cred), cred);
    assert.equal(calls, 0);
    console.log("  ✅ refreshClaudeIfNeeded skips fresh credentials");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// ─── Wire converters ───────────────────────────────────────────────────────

// 9. convertClaudeMessages maps user/assistant/toolResult roles
{
  const wire = convertClaudeMessages([
    { role: "user", content: "List files" },
    {
      role: "assistant",
      id: "turn-1",
      content: [
        { type: "text", text: "I will list files." },
        {
          type: "toolCall",
          call: { id: "toolu_1", name: "listDir", arguments: { path: "." } },
        },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [{ toolCallId: "toolu_1", content: { files: ["a.ts"] }, isError: false }],
    },
  ]);
  assert.equal(wire.length, 3);
  assert.equal(wire[0]!.role, "user");
  assert.equal(wire[1]!.role, "assistant");
  assert.equal(wire[2]!.role, "user");
  assert.deepEqual(wire[0]!.content, [{ type: "text", text: "List files" }]);
  const toolUse = wire[1]!.content[1] as Record<string, unknown>;
  assert.equal(toolUse.type, "tool_use");
  assert.equal(toolUse.id, "toolu_1");
  assert.equal(toolUse.name, "listDir");
  assert.deepEqual(toolUse.input, { path: "." });
  const toolResult = wire[2]!.content[0] as Record<string, unknown>;
  assert.equal(toolResult.type, "tool_result");
  assert.equal(toolResult.tool_use_id, "toolu_1");
  assert.ok(typeof toolResult.content === "string");
  console.log("  ✅ convertClaudeMessages maps Console → Anthropic roles");
}

// 9b. Empty tool results get a placeholder (Anthropic rejects empty content)
{
  const wire = convertClaudeMessages([
    { role: "user", content: "run it" },
    {
      role: "assistant",
      id: "turn-1",
      content: [
        { type: "toolCall", call: { id: "toolu_9", name: "bash", arguments: {} } },
        { type: "toolCall", call: { id: "toolu_9b", name: "bash", arguments: {} } },
      ],
      stopReason: "toolUse",
    },
    {
      role: "toolResult",
      results: [
        { toolCallId: "toolu_9", content: "" },
        { toolCallId: "toolu_9b", content: "   " },
      ],
    },
  ]);
  const results = wire[wire.length - 1]!.content as Array<Record<string, unknown>>;
  assert.ok(results.every((b) => typeof b.content === "string" && (b.content as string).length > 0));
  console.log("  ✅ empty tool results replaced with placeholder text");
}

// 9c. Orphaned tool results are converted to text blocks so Anthropic does not reject unexpected tool_use_id
{
  const wire = convertClaudeMessages([
    {
      role: "toolResult",
      results: [{ toolCallId: "toolu_orphaned", content: "orphan output" }],
    },
  ]);
  assert.equal(wire.length, 1);
  assert.equal(wire[0]!.role, "user");
  const block = wire[0]!.content[0] as Record<string, unknown>;
  assert.equal(block.type, "text");
  assert.match(String(block.text), /orphan output/);
  console.log("  ✅ orphaned tool results converted to safe text blocks for Claude");
}

// 9d. Assistant-only history preserves model response with user boundaries
{
  const wire = convertClaudeMessages([
    {
      role: "assistant",
      id: "turn-1",
      content: [{ type: "text", text: "I have completed step 1." }],
      stopReason: "stop",
    },
  ]);
  assert.equal(wire.length, 3);
  assert.equal(wire[0]!.role, "user");
  assert.equal(wire[1]!.role, "assistant");
  assert.equal((wire[1]!.content[0] as any).text, "I have completed step 1.");
  assert.equal(wire[2]!.role, "user");
  console.log("  ✅ assistant-only history preserves prior assistant response");
}

// 10. convertClaudeMessages merges same-role turns and keeps image attachments
{
  const wire = convertClaudeMessages([
    { role: "user", content: "Prompt 1" },
    {
      role: "assistant",
      id: "turn-1",
      content: [{ type: "text", text: "Found it." }],
      stopReason: "stop",
    },
    {
      role: "user",
      content: "",
      attachments: [{ type: "image", data: "aGVsbG8=", mimeType: "image/png" }],
    },
  ]);
  assert.equal(wire.length, 3);
  assert.equal(wire[2]!.role, "user");
  assert.equal(wire[2]!.content.length, 2);
  const image = wire[2]!.content[1] as Record<string, unknown>;
  assert.equal(image.type, "image");
  assert.deepEqual(image.source, { type: "base64", media_type: "image/png", data: "aGVsbG8=" });
  console.log("  ✅ convertClaudeMessages merges turns and maps images");
}

// 11. convertClaudeTools produces Anthropic input_schema
{
  const tools = convertClaudeTools([
    {
      name: "bash",
      description: "run a command",
      inputSchema: z.object({ command: z.string() }),
      execute: async () => ({}),
    },
  ]);
  assert.equal(tools[0]!.name, "bash");
  assert.equal(tools[0]!.description, "run a command");
  const schema = tools[0]!.input_schema as Record<string, unknown>;
  assert.equal(schema.type, "object");
  assert.ok((schema.properties as Record<string, unknown>).command);
  // Default retention caches the static tool definitions (last tool tagged).
  assert.deepEqual(tools[0]!.cache_control, { type: "ephemeral" });
  console.log("  ✅ convertClaudeTools serialises tools to input_schema");
}

// 11b. Thinking demotes to text; "none" disables all cache breakpoints
{
  const wire = convertClaudeMessages([
    { role: "user", content: "start" },
    {
      role: "assistant",
      id: "turn-1",
      content: [
        { type: "thinking", text: "let me think" },
        { type: "text", text: "done" },
      ],
      stopReason: "stop",
    },
    { role: "user", content: "go on" },
  ]);
  // Assistant thinking demotes to a text block (unsigned thinking can't replay).
  const assistant = wire.find((m) => m.role === "assistant")!;
  assert.ok(assistant.content.some((b) => b.type === "text" && b.text === "let me think"));

  const cached = convertClaudeMessages([{ role: "user", content: "hi" }]);
  const lastBlock = cached[cached.length - 1]!.content.at(-1)!;
  assert.deepEqual(lastBlock.cache_control, { type: "ephemeral" });

  const uncached = convertClaudeMessages([{ role: "user", content: "hi" }], "none");
  assert.ok(
    uncached.every((m) => m.content.every((b) => !("cache_control" in b))),
    "cacheRetention none must strip message breakpoints",
  );
  const uncachedTools = convertClaudeTools(
    [
      {
        name: "bash",
        description: "run",
        inputSchema: z.object({ command: z.string() }),
        execute: async () => ({}),
      },
    ],
    "none",
  );
  assert.ok(!("cache_control" in uncachedTools[0]!));
  console.log("  ✅ thinking demotes to text; retention none strips breakpoints");
}

// ─── Usage normalization ───────────────────────────────────────────────────

// 12. Cache hit subtracts reads from uncached input
{
  const usage = normalizeClaudeUsage(
    { input_tokens: 1000, cache_read_input_tokens: 750, cache_creation_input_tokens: 100 },
    { output_tokens: 120 },
    undefined,
  );
  assert.ok(usage);
  assert.equal(usage.input, 250);
  assert.equal(usage.cacheRead, 750);
  assert.equal(usage.cacheWrite, 100);
  assert.equal(usage.output, 120);
  assert.equal(usage.cacheStatus, "hit");
  console.log("  ✅ usage: cache hit reports read + uncached");
}

// 13. Explicit zero reads → miss; absent usage → undefined; none → unsupported
{
  const miss = normalizeClaudeUsage({ input_tokens: 500, cache_read_input_tokens: 0 }, { output_tokens: 80 }, undefined);
  assert.equal(miss?.cacheStatus, "miss");
  assert.equal(miss?.input, 500);

  assert.equal(normalizeClaudeUsage(undefined, undefined, undefined), undefined);

  const unknown = normalizeClaudeUsage({ input_tokens: 300 }, { output_tokens: 50 }, undefined);
  assert.equal(unknown?.cacheStatus, "unknown");

  const unsupported = normalizeClaudeUsage({ input_tokens: 300 }, { output_tokens: 50 }, "none");
  assert.equal(unsupported?.cacheStatus, "unsupported");
  console.log("  ✅ usage: miss / unknown / unsupported contracts");
}

// ─── SSE parsing ───────────────────────────────────────────────────────────

// 14. parseAnthropicSse pairs event names with data payloads
{
  const body = [
    "event: message_start",
    'data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}',
    "",
    "event: content_block_delta",
    'data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}',
    "data: [DONE]",
  ].join("\n");
  const events: Array<{ event: string; data: unknown }> = [];
  for await (const e of parseAnthropicSse(new Response(body))) {
    events.push(e);
  }
  assert.equal(events.length, 2);
  assert.equal(events[0]!.event, "message_start");
  assert.equal(events[1]!.event, "content_block_delta");
  assert.equal((events[1]!.data as { delta: { text: string } }).delta.text, "hi");
  console.log("  ✅ parseAnthropicSse pairs events with data");
}

// ─── Streaming ─────────────────────────────────────────────────────────────

function sseResponse(lines: string[]): Response {
  return new Response(lines.join("\n"), {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  });
}

// 15. claudeStreamFn emits text + final usage
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  let sentAuth = "";
  let sentBeta = "";
  globalThis.fetch = (async (_url: unknown, init?: { headers?: Record<string, string>; body?: string }) => {
    sentAuth = init?.headers?.Authorization ?? "";
    sentBeta = init?.headers?.["anthropic-beta"] ?? "";
    const body = JSON.parse(init?.body ?? "{}") as Record<string, unknown>;
    assert.equal(body.model, "claude-sonnet-4-6");
    assert.equal(body.stream, true);
    return sseResponse([
      "event: message_start",
      'data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":100,"cache_read_input_tokens":0}}}',
      "",
      "event: content_block_start",
      'data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}',
      "",
      "event: content_block_delta",
      'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}',
      "",
      "event: content_block_delta",
      'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}',
      "",
      "event: content_block_stop",
      'data: {"type":"content_block_stop","index":0}',
      "",
      "event: message_delta",
      'data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}',
      "",
      "event: message_stop",
      'data: {"type":"message_stop"}',
    ]);
  }) as unknown as typeof fetch;
  try {
    const deltas: Array<{ type: string; text?: string; usage?: unknown }> = [];
    for await (const d of claudeStreamFn({
      model: { id: "claude-sonnet-4-6", provider: "claude", contextWindow: 200_000 },
      systemPrompt: "Be helpful.",
      messages: [{ role: "user", content: "say hi" }],
      tools: [],
    })) {
      deltas.push(d as { type: string; text?: string });
    }
    assert.ok(sentAuth.startsWith("Bearer "), "must send OAuth bearer");
    assert.ok(sentBeta.includes("oauth-2025-04-20"), "must send OAuth beta");
    const texts = deltas.filter((d) => d.type === "text").map((d) => d.text).join("");
    assert.equal(texts, "Hello world");
    const usage = (deltas.find((d) => d.type === "usage") as { usage: Record<string, unknown> } | undefined)?.usage;
    assert.ok(usage, "expected a final usage delta");
    assert.equal(usage.input, 100);
    assert.equal(usage.output, 12);
    assert.equal(usage.cacheStatus, "miss");
    console.log("  ✅ claudeStreamFn emits text + usage with OAuth fingerprint");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// 16. claudeStreamFn accumulates tool_use input_json deltas
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    sseResponse([
      "event: message_start",
      'data: {"type":"message_start","message":{"id":"msg_2","usage":{"input_tokens":50}}}',
      "",
      "event: content_block_start",
      'data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}',
      "",
      "event: content_block_delta",
      'data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\\"command\\":"}}',
      "",
      "event: content_block_delta",
      'data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\\"ls\\"}"}}',
      "",
      "event: content_block_stop",
      'data: {"type":"content_block_stop","index":0}',
      "",
      "event: message_delta",
      'data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}',
      "",
      "event: message_stop",
      'data: {"type":"message_stop"}',
    ])) as unknown as typeof fetch;
  try {
    const toolDeltas: Array<{ id: string; name: string; argumentsJson: string }> = [];
    for await (const d of claudeStreamFn({
      model: { id: "claude-sonnet-4-6", provider: "claude", contextWindow: 200_000 },
      systemPrompt: "",
      messages: [{ role: "user", content: "run ls" }],
      tools: [],
    })) {
      if (d.type === "toolCall") toolDeltas.push(d);
    }
    assert.equal(toolDeltas[0]!.id, "toolu_1");
    assert.equal(toolDeltas[0]!.name, "bash");
    assert.equal(toolDeltas[0]!.argumentsJson, "");
    const final = toolDeltas[toolDeltas.length - 1]!;
    assert.deepEqual(JSON.parse(final.argumentsJson), { command: "ls" });
    console.log("  ✅ claudeStreamFn accumulates tool_use arguments");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// 17. claudeStreamFn surfaces error events and HTTP failures
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    sseResponse([
      "event: error",
      'data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}',
    ])) as unknown as typeof fetch;
  try {
    await assert.rejects(
      (async () => {
        for await (const _ of claudeStreamFn({
          model: { id: "claude-sonnet-4-6", provider: "claude", contextWindow: 200_000 },
          systemPrompt: "",
          messages: [{ role: "user", content: "hi" }],
          tools: [],
        })) {
          // drain
        }
      })(),
      /Overloaded/,
    );

    globalThis.fetch = (async () => new Response("unauthorized", { status: 401 })) as unknown as typeof fetch;
    await assert.rejects(
      (async () => {
        for await (const _ of claudeStreamFn({
          model: { id: "claude-sonnet-4-6", provider: "claude", contextWindow: 200_000 },
          systemPrompt: "",
          messages: [{ role: "user", content: "hi" }],
          tools: [],
        })) {
          // drain
        }
      })(),
      /Claude request failed \(401/,
    );
    console.log("  ✅ claudeStreamFn surfaces stream + HTTP errors");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// 17c. claudeStreamFn retries on 429 with retry-after header and recovers
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  let attempts = 0;
  globalThis.fetch = (async () => {
    attempts++;
    if (attempts === 1) {
      return new Response(JSON.stringify({ error: { type: "rate_limit_error" } }), {
        status: 429,
        headers: { "retry-after": "0.01", "x-should-retry": "true" },
      });
    }
    return sseResponse([
      "event: message_start",
      'data: {"type":"message_start","message":{"id":"msg_retry","usage":{"input_tokens":10}}}',
      "",
      "event: content_block_delta",
      'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"recovered"}}',
      "",
      "event: message_stop",
      'data: {"type":"message_stop"}',
    ]);
  }) as unknown as typeof fetch;
  try {
    const textDeltas: string[] = [];
    for await (const d of claudeStreamFn({
      model: { id: "claude-sonnet-4-6", provider: "claude", contextWindow: 200_000 },
      systemPrompt: "",
      messages: [{ role: "user", content: "hi" }],
      tools: [],
    })) {
      if (d.type === "text") textDeltas.push(d.text);
    }
    assert.equal(attempts, 2);
    assert.equal(textDeltas.join(""), "recovered");
    console.log("  ✅ claudeStreamFn retries on 429 and recovers");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// 17d. claudeStreamFn falls back to Haiku when Sonnet is rate-limited
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  let fallbackCapturedModel = "";
  globalThis.fetch = (async (_url: unknown, init?: { body?: string }) => {
    const body = JSON.parse(init?.body ?? "{}") as { model?: string };
    if (body.model?.includes("sonnet")) {
      return new Response(JSON.stringify({ error: { type: "rate_limit_error" } }), {
        status: 429,
        headers: { "x-should-retry": "true", "retry-after": "0.01" },
      });
    }
    fallbackCapturedModel = body.model ?? "";
    return sseResponse([
      "event: message_start",
      'data: {"type":"message_start","message":{"id":"msg_fb","usage":{"input_tokens":10}}}',
      "",
      "event: content_block_delta",
      'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"haiku response"}}',
      "",
      "event: message_stop",
      'data: {"type":"message_stop"}',
    ]);
  }) as unknown as typeof fetch;
  try {
    const textDeltas: string[] = [];
    for await (const d of claudeStreamFn({
      model: { id: "claude-sonnet-4-6", provider: "claude", contextWindow: 200_000 },
      systemPrompt: "",
      messages: [{ role: "user", content: "hi" }],
      tools: [],
    })) {
      if (d.type === "text") textDeltas.push(d.text);
    }
    assert.match(fallbackCapturedModel, /haiku/);
    assert.equal(textDeltas.join(""), "haiku response");
    console.log("  ✅ claudeStreamFn automatically falls back to Haiku on 429");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// 17b. Request body carries medium thinking + cache breakpoints
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  let capturedBody: Record<string, unknown> = {};
  globalThis.fetch = (async (_url: unknown, init?: { headers?: Record<string, string>; body?: string }) => {
    capturedBody = JSON.parse(init?.body ?? "{}") as Record<string, unknown>;
    return sseResponse([
      "event: message_start",
      'data: {"type":"message_start","message":{"id":"msg_3","usage":{"input_tokens":10}}}',
      "",
      "event: message_stop",
      'data: {"type":"message_stop"}',
    ]);
  }) as unknown as typeof fetch;
  try {
    for await (const _ of claudeStreamFn({
      model: { id: "claude-sonnet-4-5", provider: "claude", contextWindow: 200_000 },
      systemPrompt: "Be helpful.",
      messages: [{ role: "user", content: "hi" }],
      tools: [
        {
          name: "bash",
          description: "run a command",
          inputSchema: z.object({ command: z.string() }),
          execute: async () => ({}),
        },
      ],
    })) {
      // drain
    }
    assert.deepEqual(capturedBody.thinking, { type: "enabled", budget_tokens: 8192 });
    const system = capturedBody.system as Array<Record<string, unknown>>;
    assert.deepEqual(system[0]!.cache_control, { type: "ephemeral" });
    const tools = capturedBody.tools as Array<Record<string, unknown>>;
    assert.equal(tools.length, 1);
    assert.deepEqual(tools[0]!.cache_control, { type: "ephemeral" });
    const messages = capturedBody.messages as Array<{ content: Array<Record<string, unknown>> }>;
    const trailing = messages[messages.length - 1]!.content.at(-1)!;
    assert.deepEqual(trailing.cache_control, { type: "ephemeral" });
    console.log("  ✅ request body carries medium thinking + cache breakpoints");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// ─── Discovery ─────────────────────────────────────────────────────────────

// 18. fetchClaudeModels maps /v1/models entries
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({
        data: [
          {
            id: "claude-opus-4-6",
            display_name: "Claude Opus 4.6",
            max_input_tokens: 200000,
            capabilities: { image_input: { supported: true } },
          },
          { id: "claude-sonnet-4-6" },
        ],
      }),
      {
        status: 200,
        headers: { "Content-Type": "application/json" },
      },
    )) as unknown as typeof fetch;
  try {
    const models = await fetchClaudeModels();
    assert.ok(models);
    assert.deepEqual(models!.map((m) => m.id), ["claude-opus-4-6", "claude-sonnet-4-6"]);
    assert.equal(models![0]!.contextWindow, 200000);
    assert.equal(models![0]!.supportsImages, true);
    // Nullable fields fall back to registry defaults downstream.
    assert.equal(models![1]!.contextWindow, undefined);
    console.log("  ✅ fetchClaudeModels maps /v1/models entries");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// 19. fetchClaudeModels returns null on non-2xx
{
  process.env.CLAUDE_OAUTH_TOKEN = "test-oauth-token";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = (async () => new Response("nope", { status: 500 })) as unknown as typeof fetch;
  try {
    assert.equal(await fetchClaudeModels(), null);
    console.log("  ✅ fetchClaudeModels returns null on failure");
  } finally {
    globalThis.fetch = originalFetch;
    delete process.env.CLAUDE_OAUTH_TOKEN;
  }
}

// ─── Registry ──────────────────────────────────────────────────────────────

// 20. Provider catalog advertises Claude with an OAuth stream fn
{
  const { PROVIDER_CATALOG, listProviders, DEFAULT_CLAUDE_MODELS } = await import(
    "@/agent/src/commands/provider-registry.js"
  );
  assert.ok(DEFAULT_CLAUDE_MODELS.length > 0, "Claude needs a static model seed");
  assert.equal(DEFAULT_CLAUDE_MODELS[0]!.id, "claude-sonnet-4-5", "Sonnet 4.5 is the default Claude model");
  assert.equal(
    DEFAULT_CLAUDE_MODELS[0]!.contextWindow,
    1_000_000,
    "seed context matches GET /v1/models max_input_tokens",
  );

  const claudeEntry = PROVIDER_CATALOG.claude;
  assert.ok(claudeEntry, "claude should be in PROVIDER_CATALOG");
  assert.equal(claudeEntry!.name, "claude");
  assert.equal(claudeEntry!.authMethod, "oauth");
  assert.ok(typeof claudeEntry!.getStreamFn === "function");

  const listed = listProviders().find((p) => p.name === "claude");
  assert.ok(listed, "claude should be returned by listProviders()");
  console.log("  ✅ PROVIDER_CATALOG and listProviders include claude");
}

console.log("Claude provider tests passed!\n");
process.exit(0);
