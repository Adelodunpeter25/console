/**
 * Devin OAuth module tests.
 * Offline: PKCE generation, auth URL construction, JWT expiry parsing,
 * credential round-trip. The streaming implementation (Phase 3) is tested
 * separately once vendored.
 */
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  createDevinAuthorizationUrl,
  decodeJwtPayload,
  devinCredentialExists,
  exchangeDevinCode,
  generateDevinPkce,
  getTokenExpiry,
  loadDevinCredential,
  parseDevinCredential,
  refreshDevinIfNeeded,
  saveDevinCredential,
} from "@/providers/src/devin/index.js";

console.log("Running Devin provider tests...");

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
  // Within 5min skew, should be ~55 minutes from now (1h - 5min).
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
  assert.equal(decodeJwtPayload("a.b.c"), null); // 'b' is not valid base64url JSON
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
      expiresAt: Date.now() - 60_000, // 1 minute ago
    };
    await saveDevinCredential(expired);
    // Caller passes an in-memory expired credential; refresh should load fresh
    // from disk if the user has rotated it externally.
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

console.log("Devin provider tests passed!\n");
process.exit(0);
