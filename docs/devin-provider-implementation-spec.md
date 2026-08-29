# Devin Provider Implementation Specification

# Before implementing i need you to review this file  /Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/catalog/src/discovery/devin-proto.ts

## Overview

This specification outlines the complete implementation of Devin (Codeium/Windsurf) as a provider in Console. Devin uses PKCE OAuth authentication and the Connect streaming protocol with protobuf encoding, as implemented in the oh-my-pi project.

**Reference Implementation**: `/Users/adelodunpeter/Developer/Projects/oh-my-pi/packages/ai/src/`
- OAuth: `registry/oauth/devin.ts`
- Provider Registry: `registry/devin.ts`  
- Streaming: `providers/devin.ts`

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
- Update `AuthStatusResponse` to include Devin status
- Add Devin-specific types if needed (session tokens, JWT metadata)

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
- Add Devin to `fetchModelsForProvider` function
- Update `listProviders` and `getProvider` functions

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

**Reference**: Existing OAuth routes for gemini/antigravity/codex

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

**Key Functions**:
```typescript
export const streamDevin: StreamFunction<"devin"> = (
  model: Model<"devin">,
  context: Context,
  options?: DevinOptions,
): AssistantMessageEventStream
```

**Protocol Details**:
- Base URL: `https://server.codeium.com`
- Chat endpoint: `/exa.api_server_pb.ApiServerService/GetChatMessage`
- Auth endpoint: `/exa.auth_pb.AuthService/GetUserJwt`
- Content-Type: `application/connect+proto`
- Encoding: gzip with connect-protocol-version: 1

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

#### 3.4 Add Protobuf Dependencies
**File**: `apps/server/package.json`

**Tasks**:
- Add protobuf library dependency
- Add gzip dependencies if not present
- Add any required Devin protobuf definitions

**Reference**: `packages/ai/src/providers/devin.ts` imports from `@oh-my-pi/pi-catalog/discovery/devin-proto`

## Phase 4: Desktop Integration

### Tasks

#### 4.1 Update Desktop Auth Types
**File**: `apps/desktop/crates/console-core/src/types/auth.rs`

**Tasks**:
- Add Devin to `OAuthProviderId` enum
- Update `AuthStatusResponse` to include Devin
- Remove any codebuff-specific types if present

**Reference**: Console's existing auth types

#### 4.2 Implement Desktop OAuth Flow
**File**: `apps/desktop/src/state/auth.rs`

**Tasks**:
- Add Devin to `login_provider` function
- Implement PKCE OAuth flow for desktop
- Handle callback server on port 59653
- Update auth status refresh to include Devin

**Reference**: `packages/ai/src/registry/oauth/devin.ts` (OAuth flow)
**Reference**: Console's existing OAuth implementations (gemini/antigravity)

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
- Add Devin to OAuth login flow
- Update callback handling for Devin
- Remove any codebuff-specific code

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
**Reference**: Console's existing provider tests (codebuff-provider.test.ts as template)

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

- Protobuf library for encoding/decoding
- Gzip compression library
- PKCE implementation (can use existing or adapt from oh-my-pi)
- JWT parsing library

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
2. **Existing Providers**: `apps/server/providers/src/` (gemini, antigravity, opencode, codex)
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