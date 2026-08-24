/**
 * Unit Tests for the Codebuff (Freebuff) provider.
 * Operates offline — uses mock fetch responses (0 API credits consumed).
 *
 * Covers:
 *  1. startCodebuffLogin → POST /api/auth/cli/code → loginUrl + polling params
 *  2. pollCodebuffLogin → GET /api/auth/cli/status → authToken credential saved
 *  3. codebuffStreamFn → text / thinking / toolCall deltas via mock SSE
 *  4. streamFn rejects without a credential
 *  5. provider-registry catalog entry for "codebuff"
 *  6. codebuff free-model catalog (ids, context windows)
 */
import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import type { AgentMessage, AgentTool } from "@console/types";
import { z } from "zod";

import { listModelsForProvider, listProviders } from "@/agent/src/commands/provider-registry.js";
import {
  startCodebuffLogin,
  pollCodebuffLogin,
  codebuffStreamFn,
  loadCodebuffCredential,
  CODEBUFF_MODEL_SPECS,
} from "@/providers/src/index.js";

console.log("Running Codebuff (Freebuff) provider tests...");

// Point credential writes at a temp dir so tests never touch the real
// ~/.console/codebuff-creds.json.
let tempDir: string | null = null;
async function withTempCreds<T>(fn: () => Promise<T>): Promise<T> {
  tempDir = await mkdtemp(join(tmpdir(), "codebuff-test-"));
  process.env.CODEBUFF_CREDENTIALS_PATH = join(tempDir, "codebuff-creds.json");
  delete process.env.CODEBUFF_API_KEY;
  try {
    return await fn();
  } finally {
    delete process.env.CODEBUFF_CREDENTIALS_PATH;
    delete process.env.CODEBUFF_API_KEY;
    if (tempDir) await rm(tempDir, { recursive: true, force: true });
    tempDir = null;
  }
}

const ORIGINAL_FETCH = globalThis.fetch;

function mockFetch(handler: (url: string, init?: RequestInit) => Promise<Response>) {
  globalThis.fetch = (async (input: any, init?: RequestInit) =>
    handler(String(input), init)) as typeof fetch;
}

function restoreFetch() {
  globalThis.fetch = ORIGINAL_FETCH;
}

// 1. startCodebuffLogin — POST /api/auth/cli/code
{
  await withTempCreds(async () => {
    mockFetch(async (url, init) => {
      assert.ok(url.endsWith("/api/auth/cli/code"), `unexpected url ${url}`);
      assert.equal(init?.method, "POST");
      const body = JSON.parse(String(init?.body));
      assert.ok(typeof body.fingerprintId === "string" && body.fingerprintId.length > 0);
      return new Response(
        JSON.stringify({
          loginUrl: "https://codebuff.com/login?code=abc",
          fingerprintHash: "hash-1",
          expiresAt: "2030-01-01T00:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    });

    try {
      const login = await startCodebuffLogin();
      assert.equal(login.loginUrl, "https://codebuff.com/login?code=abc");
      assert.equal(login.fingerprintHash, "hash-1");
      assert.equal(login.expiresAt, "2030-01-01T00:00:00Z");
      assert.ok(login.fingerprintId.startsWith("console-"));
      console.log("  ✅ startCodebuffLogin → login code + polling params");
    } finally {
      restoreFetch();
    }
  });
}

// 2. pollCodebuffLogin — GET /api/auth/cli/status → credential saved
{
  await withTempCreds(async () => {
    mockFetch(async (url) => {
      assert.ok(url.includes("/api/auth/cli/status"), `unexpected url ${url}`);
      assert.ok(url.includes("fingerprintId=console-test"));
      return new Response(
        JSON.stringify({
          user: { id: "u-1", name: "Test User", email: "test@example.com", authToken: "token-123" },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    });

    try {
      const status = await pollCodebuffLogin({
        fingerprintId: "console-test",
        fingerprintHash: "hash-1",
        expiresAt: "2030-01-01T00:00:00Z",
      });
      assert.equal(status.loggedIn, true);
      assert.equal(status.credential?.authToken, "token-123");
      assert.equal(status.credential?.email, "test@example.com");

      const loaded = await loadCodebuffCredential();
      assert.equal(loaded?.authToken, "token-123");
      console.log("  ✅ pollCodebuffLogin → authToken persisted");
    } finally {
      restoreFetch();
    }
  });
}

// 2b. pollCodebuffLogin returns loggedIn:false while the user hasn't approved
{
  await withTempCreds(async () => {
    mockFetch(async () =>
      new Response(JSON.stringify({}), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    try {
      const status = await pollCodebuffLogin({
        fingerprintId: "console-test",
        fingerprintHash: "hash-1",
        expiresAt: "2030-01-01T00:00:00Z",
      });
      assert.equal(status.loggedIn, false);
      assert.equal(status.credential, undefined);
      console.log("  ✅ pollCodebuffLogin → not-yet-approved returns loggedIn:false");
    } finally {
      restoreFetch();
    }
  });
}

// 2c. pollCodebuffLogin treats 401 (unapproved) as loggedIn:false, not an error
// (the real Codebuff API returns 401 "Authentication failed" until approval).
{
  await withTempCreds(async () => {
    mockFetch(async () =>
      new Response(JSON.stringify({ error: "Authentication failed" }), {
        status: 401,
        headers: { "Content-Type": "application/json" },
      }),
    );
    try {
      const status = await pollCodebuffLogin({
        fingerprintId: "console-test",
        fingerprintHash: "hash-1",
        expiresAt: "2030-01-01T00:00:00Z",
      });
      assert.equal(status.loggedIn, false);
      console.log("  ✅ pollCodebuffLogin → 401 maps to loggedIn:false (keep polling)");
    } finally {
      restoreFetch();
    }
  });
}

// 3. codebuffStreamFn — SSE deltas (mock fetch) + agent-run lifecycle
{
  await withTempCreds(async () => {
    await writeFile(process.env.CODEBUFF_CREDENTIALS_PATH!, JSON.stringify({ authToken: "tok" }));

    const mockSse = [
      'data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":""}}]}',
      "",
      'data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"Let me think"}}]}',
      "",
      'data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"pineapple"}}]}',
      "",
      'data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"list_directory","arguments":"{\\"path\\":"}}]}}]}',
      "",
      'data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\".\\"}"}}]}}]}',
      "",
      'data: {"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}',
      "",
      "data: [DONE]",
      "",
    ].join("\n");

    const startedRuns: string[] = [];
    const finishedRuns: Array<{ runId: string; status: string }> = [];
    const sessionPosts: string[] = [];

    mockFetch(async (url, init) => {
      if (url.endsWith("/api/v1/freebuff/session")) {
        const rawHeaders = init?.headers as Record<string, string> | undefined;
        assert.equal(rawHeaders?.["x-freebuff-model"], "deepseek/deepseek-v4-flash");
        sessionPosts.push("1");
        return new Response(
          JSON.stringify({
            status: "active",
            accessTier: "full",
            instanceId: "inst-1",
            model: "deepseek/deepseek-v4-flash",
            admittedAt: new Date().toISOString(),
            expiresAt: new Date(Date.now() + 3_600_000).toISOString(),
            remainingMs: 3_600_000,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }

      if (url.endsWith("/api/v1/agent-runs")) {
        const body = JSON.parse(String(init?.body));
        if (body.action === "START") {
          assert.equal(body.agentId, "base2-free-deepseek-flash");
          const runId = "run-abc";
          startedRuns.push(runId);
          return new Response(JSON.stringify({ runId }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (body.action === "FINISH") {
          finishedRuns.push({ runId: body.runId, status: body.status });
          return new Response(JSON.stringify({}), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        throw new Error(`Unexpected agent-runs action ${body.action}`);
      }

      assert.ok(url.endsWith("/api/v1/chat/completions"), `unexpected url ${url}`);
      const rawHeaders = init?.headers as
        | Record<string, string>
        | Headers
        | undefined;
      const authHeader = rawHeaders instanceof Headers
        ? rawHeaders.get("Authorization")
        : (rawHeaders?.["Authorization"] ?? rawHeaders?.["authorization"]);
      assert.equal(authHeader, "Bearer tok");
      const body = JSON.parse(String(init?.body));
      assert.equal(body.model, "deepseek/deepseek-v4-flash");
      // Free-tier metadata: cost_mode free, server-issued run_id, and the
      // live freebuff session instance id (CLI handshake).
      assert.equal(body.codebuff_metadata?.cost_mode, "free");
      assert.equal(body.codebuff_metadata?.run_id, "run-abc");
      assert.equal(body.codebuff_metadata?.freebuff_instance_id, "inst-1");
      // The system prompt must open with the canonical Buffy marker or the
      // server 403s with free_mode_cli_required (requestHasFreebuffSystemMarker).
      const systemMsg = body.messages?.find((m: any) => m.role === "system");
      assert.ok(
        systemMsg && systemMsg.content.startsWith("You are Buffy, the strategic coding assistant."),
        `system prompt must open with the Buffy marker, got: ${JSON.stringify(body.messages)}`,
      );
      // Console tools must be sent under freebuff signature names so the
      // server's foreign_toolset downgrade doesn't fire.
      const toolNames = (body.tools ?? []).map((t: any) => t.function?.name);
      assert.ok(
        toolNames.includes("list_directory"),
        `listDir should be aliased to list_directory, got: ${JSON.stringify(toolNames)}`,
      );
      return new Response(mockSse, {
        headers: { "Content-Type": "text/event-stream" },
      });
    });

    try {
      const deltas: Array<{ type: string; text?: string; name?: string; argumentsJson?: string }> = [];
      for await (const delta of codebuffStreamFn({
        model: { id: "deepseek/deepseek-v4-flash", provider: "codebuff", contextWindow: 400_000 },
        systemPrompt: "Be terse.",
        messages: [{ role: "user", content: "hi" }],
        tools: [{ name: "listDir", description: "List a directory", inputSchema: z.object({ path: z.string() }) }],
      })) {
        deltas.push(delta);
      }

      // Lifecycle: session handshake → START registered a run, stream consumed it,
      // FINISH closed it.
      assert.deepEqual(sessionPosts, ["1"]);
      assert.deepEqual(startedRuns, ["run-abc"]);
      assert.deepEqual(finishedRuns, [{ runId: "run-abc", status: "completed" }], "run must be finished after streaming");
      assert.deepEqual(deltas.filter((d) => d.type === "text").map((d) => (d as { text: string }).text).join(""), "pineapple");

      const thinkingDeltas = deltas.filter((d) => d.type === "thinking");
      const textDeltas = deltas.filter((d) => d.type === "text");
      const toolDeltas = deltas.filter((d) => d.type === "toolCall");

      assert.ok(thinkingDeltas.length > 0, "should emit thinking deltas");
      assert.equal(thinkingDeltas.map((d) => (d as { text: string }).text).join(""), "Let me think");
      assert.equal(textDeltas.map((d) => (d as { text: string }).text).join(""), "pineapple");

      assert.ok(toolDeltas.length > 0, "should emit toolCall deltas");
      const toolStart = toolDeltas.find((d) => d.name === "listDir") as { id: string; name: string; argumentsJson: string } | undefined;
      assert.ok(toolStart, "freebuff signature tool name must reverse-map back to the console tool id (list_directory → listDir)");
      const assembled = toolDeltas
        .filter((d) => (d as { id: string }).id === toolStart.id)
        .map((d) => (d as { argumentsJson: string }).argumentsJson)
        .join("");
      assert.deepEqual(JSON.parse(assembled), { path: "." }, `args were ${assembled}`);
      console.log("  ✅ codebuffStreamFn streaming (thinking/text/toolCall + free metadata + run lifecycle)");
    } finally {
      restoreFetch();
    }
  });
}

// 4. streamFn rejects when no credential is available
{
  await withTempCreds(async () => {
    delete process.env.CODEBUFF_API_KEY;
    let threw = false;
    try {
      for await (const _delta of codebuffStreamFn({
        model: { id: "deepseek/deepseek-v4-flash", provider: "codebuff", contextWindow: 400_000 },
        systemPrompt: "hi",
        messages: [{ role: "user", content: "hi" }],
        tools: [],
      })) {
        // no-op
      }
    } catch (err) {
      threw = true;
      assert.ok((err as Error).message.includes("not logged in"));
    }
    assert.ok(threw, "streamFn should throw without a credential");
    console.log("  ✅ codebuffStreamFn rejects without credentials");
  });
}

// 5. Provider-registry catalog entry for "codebuff"
{
  const providers = listProviders();
  const entry = providers.find((p) => p.name === "codebuff");
  assert.ok(entry, "codebuff should be registered in the provider catalog");
  assert.equal(entry!.displayName, "Codebuff (Freebuff)");

  const models = listModelsForProvider("codebuff");
  assert.ok(models.length > 0);
  assert.ok(models.some((m) => m.id === "deepseek/deepseek-v4-flash"));
  assert.ok(models.some((m) => m.id === "mimo/mimo-v2.5"));
  assert.ok(models.every((m) => m.provider === "codebuff"));
  console.log("  ✅ provider-registry codebuff catalog entry");
}

// 6. Free-tier model catalog metadata
{
  assert.ok(CODEBUFF_MODEL_SPECS.length >= 2);
  const flash = CODEBUFF_MODEL_SPECS.find((m) => m.id === "deepseek/deepseek-v4-flash");
  assert.ok(flash && flash.premium === false, "deepseek-v4-flash is a non-premium free model");
  const kimi = CODEBUFF_MODEL_SPECS.find((m) => m.id === "moonshotai/kimi-k2.7-code");
  assert.equal(kimi?.contextWindow, 250_000, "Kimi uses a 250k context window");
  console.log("  ✅ codebuff free-tier model catalog metadata");
}

console.log("Codebuff (Freebuff) provider tests passed!\n");
