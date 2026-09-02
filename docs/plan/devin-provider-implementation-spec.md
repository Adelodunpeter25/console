# Devin Provider Implementation Specification

## Overview

This specification outlines the complete implementation of Devin (Codeium/Windsurf) as a provider in Console. Devin uses PKCE OAuth authentication and the Connect streaming protocol with protobuf encoding, as implemented in the oh-my-pi project.

**Reference Implementation**: `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/ai/src/`
- OAuth: `registry/oauth/devin.ts`
- Provider Registry: `registry/devin.ts`  
- Streaming: `providers/devin.ts`
- Protobuf definitions: `packages/catalog/src/discovery/devin-proto.ts` (2037-line generated codec)
- Protobuf runtime: `packages/catalog/src/discovery/protobuf.ts` (`create`/`toBinary`/`fromBinary`/`pb`)
- Model discovery: `packages/catalog/src/discovery/devin.ts` (`fetchDevinModels` via `GetCliModelConfigs`)

### Important: Two API domains

Devin/Codeium spans **two different hosts** — don't conflate them:

| Purpose | Host | Notes |
|---|---|---|
| OAuth login + token exchange | `app.devin.ai` / `api.devin.ai` | PKCE, returns a session token |
| Chat streaming (`GetChatMessage`), JWT (`GetUserJwt`), model discovery (`GetCliModelConfigs`) | `server.codeium.com` | Codeium/Windsurf Cascade Connect API |

The OAuth flow returns a session token; the streaming path first exchanges it for a
short-lived **user JWT** via `GetUserJwt` (protobuf), which may also return a
`customApiServerUrl` that overrides the chat base URL. **Verify before implementing**
that a token from `api.devin.ai/auth/cli/token` is accepted by `server.codeium.com` —
this is the riskiest assumption in the whole integration.

### Console cannot depend on `@oh-my-pi/pi-catalog`

The proto definitions and runtime must be **vendored** into Console
(~3,100 lines total) — copy `devin-proto.ts` + `protobuf.ts` into
`apps/server/providers/src/devin/proto/` and adapt imports. Do not add the
oh-my-pi package as a dependency.

## Architecture Overview

Devin integration requires:
1. **PKCE OAuth Authentication** - Secure browser-based login flow
2. **Connect Protocol Streaming** - Protobuf/gzip streaming for real-time responses
3. **Session Management** - JWT-based session tokens
4. **Tool Calling Support** - Native tool call streaming
5. **Thinking Blocks** - Advanced reasoning display

## Phase 1: Type Definitions and Provider Registry

### Tasks

#### 1.1 Update Type Definitions
**Files**: `packages/types/src/model.ts`, `packages/types/src/api.ts`

**Tasks**:
- Add `"devin"` to `ProviderId` type
- Add Devin to `OAuthProviderId` type (uses OAuth flow)
- `AuthStatusResponse` is `Record<OAuthProviderId, ProviderAuthStatus>` (`packages/types/src/api.ts`), so it extends automatically — but the server's `AuthService.getAuthStatus()` returns an explicit object and must add a `devin` key
- Add Devin-specific types if needed (session tokens, JWT metadata)
- **Ripple effect**: adding to `ProviderId` forces exhaustiveness updates everywhere (usage route gating in `apps/server/api/src/routes/usage.ts`, desktop Rust `OAuthProviderId` enum in `apps/desktop/crates/console-core/src/types/auth.rs`, mobile `INITIAL_STATUS` in `apps/mobile/stores/useAuthStore.ts`)

**Reference**: `packages/ai/src/registry/devin.ts` (provider definition)

#### 1.2 Create Provider Registry Entry
**File**: `apps/server/agent/src/commands/provider-registry.ts`

**Tasks**:
- Add `DEFAULT_DEVIN_MODELS` array with available Devin models
- Add Devin entry to `PROVIDER_CATALOG` with:
  - `name: "devin"`
  - `displayName: "Devin"`
  - `description: "Codeium/Windsurf AI with advanced reasoning and tool calling"`
  - `authMethod: "oauth"`
  - `models: DEFAULT_DEVIN_MODELS`
  - `getStreamFn: () => devinStreamFn`
- Add a **Devin branch to `fetchModelsForProvider`** — do NOT use the generic
  `fetchAvailableModels`. Devin discovery is the protobuf `GetCliModelConfigs`
  unary RPC against `server.codeium.com` (`application/proto`, no Connect
  streaming framing, gunzip fallback), ported from oh-my-pi's
  `packages/catalog/src/discovery/devin.ts` (`fetchDevinModels`). It requires a
  stored credential (session token in `Metadata.apiKey` with the
  `devin-session-token$` prefix) and normalizes `ClientModelConfig` entries —
  skip `disabled` ones, map `modelUid` → id, `label` → name,
  `supportsImages`, `maxTokens` → context window.
- `listProviders`/`getProvider` need no changes (`PROVIDER_CATALOG` is a
  `Record<ProviderId, ProviderEntry>` — the catalog entry alone covers them)

**Reference**: `packages/ai/src/registry/devin.ts` (provider structure)

## Phase 2: OAuth Authentication Implementation

### Tasks

#### 2.1 Create Devin OAuth Module
**New File**: `apps/server/providers/src/devin/oauth.ts`

**Tasks**:
- Implement PKCE code verifier/challenge generation
- Create OAuth login URL generation with Devin parameters:
  - Redirect URI configuration
  - State parameter for CSRF protection
  - `code_challenge` and `code_challenge_method: S256`
  - `prompt: "select_account"`
- Implement authorization code exchange for session token
- Parse JWT expiry from token payload
- Implement token refresh logic if needed

**Reference**: `packages/ai/src/registry/oauth/devin.ts` (complete OAuth flow)

**Key Constants**:
```typescript
const DEVIN_WEBAPP_URL = "https://app.devin.ai";
const DEVIN_API_URL = "https://api.devin.ai";
const CALLBACK_PORT = 59653;
const CALLBACK_PATH = "/callback";
const TOKEN_PATH = "/auth/cli/token";
```

#### 2.2 Add OAuth Routes
**File**: `apps/server/api/src/routes/auth.ts`

**Tasks**:
- Add `POST /api/auth/login/url` support for Devin provider
- Add `POST /api/auth/login/callback` support for Devin provider
- Update provider validation to include "devin"

**Reference**: Existing OAuth routes for antigravity/codex

#### 2.3 Implement Server-Side OAuth Service
**File**: `apps/server/api/src/services/auth.service.ts`

**Tasks**:
- Add Devin OAuth configuration constants
- Implement `getLoginUrl` for Devin (PKCE flow)
- Implement `handleCallback` for Devin (code exchange)
- Add Devin credential storage logic
- Update `getAuthStatus` to include Devin credential check

**Reference**: `packages/ai/src/registry/oauth/devin.ts` (OAuth implementation)

#### 2.4 Update Provider Index
**File**: `apps/server/providers/src/index.ts`

**Tasks**:
- Export Devin OAuth functions
- Export Devin types and constants
- Add Devin to the main provider exports

**Reference**: `packages/ai/src/registry/devin.ts` (provider exports)

## Phase 3: Streaming Implementation

### Tasks

#### 3.1 Create Devin Streaming Module
**New File**: `apps/server/providers/src/devin/stream-fn.ts`

**Tasks**:
- Implement Connect protocol streaming with protobuf encoding
- Add gzip compression/decompression support
- Implement frame parsing with length prefixes
- Handle streaming text, thinking blocks, and tool calls
- Implement usage tracking (tokens, cache read/write)
- Add error handling for Connect trailers
- Implement context overflow recovery logic

**Reference**: `packages/ai/src/providers/devin.ts` (streaming implementation)

> **Adaptation required**: oh-my-pi's `streamDevin` is a
> `StreamFunction<"devin-agent">` returning its own `AssistantMessageEventStream`.
> Console's provider contract is `StreamFn`
> (`apps/server/agent/src/service/types.ts`) which returns
> `EventStream<AgentSessionEvent, AgentMessage[]>` — the protocol logic ports,
> but the event emission must be rewritten to Console's event shapes
> (`modelStreamPart` deltas, `toolCall` previews) as consumed by
> `stream-turn.ts` / the `RunEventHub`.

**Key Functions**:
```typescript
export const streamDevin: StreamFunction<"devin"> = (
  model: Model<"devin">,
  context: Context,
  options?: DevinOptions,
): AssistantMessageEventStream
```

**Protocol Details**:
- Base URL: `https://server.codeium.com` (chat), not `api.devin.ai` (OAuth) — see "Two API domains" above
- Chat endpoint: `/exa.api_server_pb.ApiServerService/GetChatMessage`
- Auth endpoint: `/exa.auth_pb.AuthService/GetUserJwt`
- Content-Type: `application/connect+proto`
- Connect **streaming framing**: 1 flag byte (`0x01` = gzip payload, `0x02` = end-of-stream JSON trailers) + 4-byte big-endian length prefix + payload. Enforce a max frame cap (oh-my-pi uses 16 MiB) — the length prefix is untrusted input
- Request bodies are gzip-compressed protobuf (`connect-content-encoding: gzip`)
- Errors often arrive as Connect trailers with HTTP 200 — parse the end-of-stream trailer JSON (`{ error: { code, message } }`), don't rely on response status
- Context-overflow recovery: treat `invalid_argument` + "internal error" trailers on large histories (≥512 KB of shrinkable prompts) as context overflow, mirroring oh-my-pi's heuristic

#### 3.2 Implement Auth Metadata Fetching
**New File**: `apps/server/providers/src/devin/auth.ts`

**Tasks**:
- Implement JWT fetching via `/GetUserJwt` endpoint
- Handle session token normalization (`devin-session-token$` prefix)
- Parse custom API server URL from auth response
- Implement IDE metadata headers ( Windsurf, version info)

**Reference**: `packages/ai/src/providers/devin.ts` (fetchDevinAuthMetadata function)

**Metadata Headers**:
```typescript
ideName: "windsurf"
ideVersion: "3.2.23"
extensionName: "windsurf"
extensionVersion: "1.48.2"
locale: "en"
```

#### 3.3 Implement Message Transformation
**New File**: `apps/server/providers/src/devin/transform-messages.ts`

**Tasks**:
- Transform Console message format to Devin chat message prompts
- Handle system prompt normalization
- Map tool definitions to Devin tool schema
- Implement conversation ID/session ID management
- Handle stop patterns and configuration

**Reference**: `packages/ai/src/providers/devin.ts` (buildDevinChatRequest function)

#### 3.4 Vendor Protobuf Definitions and Runtime
**New Files**: `apps/server/providers/src/devin/proto/devin-proto.ts`, `apps/server/providers/src/devin/proto/protobuf.ts`

**Tasks**:
- Copy oh-my-pi's `packages/catalog/src/discovery/devin-proto.ts` (generated
  message codecs: `GetChatMessageRequest/Response`, `Metadata`,
  `ChatMessagePrompt`, `ChatToolCall/Definition`, `CompletionConfiguration`,
  `GetUserJwtRequest/Response`, `GetCliModelConfigsRequest/Response`, enums)
  and its `protobuf.ts` runtime into `apps/server/providers/src/devin/proto/`
  and rewrite imports
- Note: proto `uint64` fields (`Metadata.requestId`, cost fields) are TS
  `bigint` — keep them out of any JSON-serialized SSE events
- Gzip is already available via `node:zlib` (`gzipSync`/`gunzipSync`) — no new
  dependency needed

## Phase 4: Desktop Integration

### Tasks

#### 4.1 Update Desktop Auth Types
**File**: `apps/desktop/crates/console-core/src/types/auth.rs`

**Tasks**:
- Add `Devin` to `OAuthProviderId` enum (`as_str()` returns `"devin"`)
- Update `AuthStatusResponse` to include a `devin` field
- Add a `"devin"` match arm to `login_provider` in `apps/desktop/src/state/auth.rs`

#### 4.2 Implement Desktop OAuth Flow
**File**: `apps/desktop/src/state/auth.rs`

**Tasks**:
- Add `"devin"` to the `login_provider` match in `apps/desktop/src/state/auth.rs`
- No new OAuth code needed: the desktop already fetches the login URL from
  `POST /api/auth/login/url`, parses the port from the returned `redirect_uri`,
  binds a local `TcpListener`, catches the redirect, and POSTs the code back to
  `/api/auth/login/callback`. Returning redirect port 59653 from the server's
  Devin `getLoginUrl` is all the desktop needs
- Update auth status refresh (automatic via `getAuthStatus` once the server
  includes `devin`)

#### 4.3 Update Desktop UI Components
**Files**: Desktop UI components

**Tasks**:
- Add Devin icon to provider icons
- Update accounts page to show Devin provider
- Add Devin login button and status display
- Update model picker to include Devin models

**Reference**: Console's existing provider UI components

## Phase 5: Mobile Integration

### Tasks

#### 5.1 Update Mobile Auth Store
**File**: `apps/mobile/stores/useAuthStore.ts`

**Tasks**:
- Add Devin to `INITIAL_STATUS`
- Implement Devin OAuth login for mobile
- Add Devin to project ID handling if needed
- Update auth status loading to include Devin

**Reference**: Console's existing mobile auth implementations

#### 5.2 Update Mobile Auth Hooks
**File**: `apps/mobile/hooks/useAuth.ts`

**Tasks**:
- Add Devin to the OAuth login flow
- Update callback handling for Devin

> **Mobile caveat**: Devin's OAuth redirects to the loopback URI
> `http://127.0.0.1:59653/callback`, which a phone cannot receive — the
> redirect lands on nothing. Mobile's flow relies on an app deep link
> (`scheme://auth?code=…&state=…` via `expo-linking`). Verify whether
> `app.devin.ai` supports a custom redirect URI for the CLI flow; if not,
> mobile needs a **manual code paste** fallback (oh-my-pi's registry marks
> devin with `pasteCodeFlow: true` — likely for exactly this reason).

**Reference**: Console's existing mobile auth hooks

#### 5.3 Update Mobile UI Components
**Files**: Mobile UI components

**Tasks**:
- Add Devin icon to provider icons
- Update account settings to show Devin provider
- Add Devin login button and status display
- Update deep link handling for Devin OAuth

**Reference**: Console's existing mobile provider UI

## Phase 6: Testing and Validation

### Tasks

#### 6.1 Create Devin Provider Tests
**New File**: `apps/server/tests/devin-provider.test.ts`

**Tasks**:
- Test OAuth login flow
- Test token exchange
- Test streaming functionality
- Test error handling
- Test tool calling integration
- Test thinking blocks

**Reference**: `packages/ai/src/providers/devin.ts` (streaming logic)
**Reference**: Console's existing provider tests (`codex-provider.test.ts` and
`opencode.test.ts` are the templates — mock the fetch/protobuf transport and
assert wire-level details; note the old `codebuff-provider.test.ts` template no
longer exists, codebuff was removed)

#### 6.2 Integration Testing
**Tasks**:
- Test end-to-end OAuth flow on desktop
- Test end-to-end OAuth flow on mobile
- Test streaming with actual Devin API
- Test tool calling with Devin
- Test error scenarios (invalid tokens, rate limits, etc.)

#### 6.3 Performance Testing
**Tasks**:
- Benchmark streaming performance
- Test memory usage with large conversations
- Test context overflow recovery
- Validate gzip compression benefits

## Phase 7: Documentation and Deployment

### Tasks

#### 7.1 Update Documentation
**Tasks**:
- Update README with Devin provider info
- Add Devin setup instructions
- Document OAuth flow for users
- Add troubleshooting guide

#### 7.2 Update Configuration
**Tasks**:
- Update any configuration files with Devin settings
- Add Devin to default provider lists
- Update environment variable documentation

#### 7.3 Deployment Preparation
**Tasks**:
- Ensure all dependencies are properly packaged
- Test with production build
- Verify mobile bundling works
- Prepare release notes

## Implementation Notes

### Key Differences from Existing Providers

1. **Protocol**: Devin uses Connect/protobuf instead of REST/JSON
2. **Compression**: Gzip compression required for requests/responses
3. **Authentication**: JWT-based in addition to session tokens
4. **Port**: Uses port 59653 for OAuth callback (vs existing ports)
5. **Session Format**: Requires `devin-session-token$` prefix

### Dependencies Required

- **Vendored protobuf codec** — `devin-proto.ts` + `protobuf.ts` copied from
  oh-my-pi (no external protobuf runtime; `node:zlib` covers gzip)
- PKCE implementation (adapt from oh-my-pi's `registry/oauth/pkce.ts` — ~20 lines
  using Web Crypto)
- JWT parsing is just base64url + `JSON.parse` on the payload (see
  `getTokenExpiry` in oh-my-pi's `registry/oauth/devin.ts`) — no library needed

### Risk Mitigation

1. **Fallback Handling**: Implement graceful degradation if protobuf parsing fails
2. **Error Recovery**: Use oh-my-pi's context overflow recovery pattern
3. **Rate Limiting**: Add rate limit detection and handling
4. **Session Management**: Implement proper token refresh logic
5. **Backward Compatibility**: Ensure existing providers continue to work

## Success Criteria

- [ ] Devin provider appears in provider list on all platforms
- [ ] OAuth login works on desktop and mobile
- [ ] Streaming responses work correctly
- [ ] Tool calling functions properly
- [ ] Thinking blocks display correctly
- [ ] Error handling is robust
- [ ] Tests pass for all Devin functionality
- [ ] Documentation is complete
- [ ] Performance is acceptable

## References

### Oh-My-Pi Implementation Files

1. **OAuth Implementation**: `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/ai/src/registry/oauth/devin.ts`
   - PKCE flow implementation
   - Token exchange logic
   - JWT parsing

2. **Provider Registry**: `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/ai/src/registry/devin.ts`
   - Provider definition structure
   - Lazy loading pattern

3. **Streaming Implementation**: `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/ai/src/providers/devin.ts`
   - Connect protocol implementation
   - Protobuf encoding/decoding
   - Error handling and recovery
   - Usage tracking

### Console Implementation Patterns

1. **Existing OAuth**: `apps/server/api/src/routes/auth.ts`, `apps/server/api/src/services/auth.service.ts`
2. **Existing Providers**: `apps/server/providers/src/` (antigravity, opencode, codex)
3. **Desktop Auth**: `apps/desktop/src/state/auth.rs`
4. **Mobile Auth**: `apps/mobile/stores/useAuthStore.ts`, `apps/mobile/hooks/useAuth.ts`

## Timeline Estimate

- **Phase 1**: 2-3 hours (Type definitions and registry)
- **Phase 2**: 4-6 hours (OAuth implementation)
- **Phase 3**: 8-12 hours (Streaming implementation - most complex)
- **Phase 4**: 3-4 hours (Desktop integration)
- **Phase 5**: 3-4 hours (Mobile integration)
- **Phase 6**: 4-6 hours (Testing and validation)
- **Phase 7**: 2-3 hours (Documentation and deployment)

**Total Estimate**: 26-38 hours for complete implementation