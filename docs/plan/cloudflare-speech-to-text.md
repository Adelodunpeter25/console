# Speech-to-Text via Cloudflare Workers AI

*Written: 2026-11-09. Context: relay folder being deleted; STT will be called directly from server-go.*

## Goal

Add voice transcription to the desktop (Rust) + Android (Kotlin) apps: the user records speech, the audio is transcribed to text, and the transcript is returned to the caller (for voice notes / dictation into the composer).

## Model choice

**`@cf/openai/whisper-large-v3-turbo`** — best speed/accuracy tradeoff on Cloudflare's catalog:

- Near-whisper quality (~5% WER clean / ~8% noisy), multilingual
- Turbo variant = noticeably lower latency than full `@cf/openai/whisper`
- Pricing: $0.0005/audio-minute ≈ 46.63 neurons/min
- Free tier: 10,000 neurons/day → **~215 minutes of audio per day free** (resets 00:00 UTC)
- Endpoint: `POST https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/run/@cf/openai/whisper-large-v3-turbo`
  - Auth: `Authorization: Bearer <CF_API_TOKEN>` (account-scoped token with Workers AI permission)
  - Body: binary audio (`Content-Type: audio/wav`, etc.), optional `multipart/form-data` with prompt fields
  - Response: `{ "result": { "text": "..." }, "success": true, ... }`

Not in scope: Deepgram nova-3/flux (streaming; ~10x neuron cost), Groq fallback (can add later), on-device whisper.cpp.

## Phases

### Phase 1 — Transcription service in server-go
- New package `apps/server-go/internal/services/stt/` (or `speech/`):
  - `Client` struct holding `accountID`, `apiToken`, `model` (constant default `@cf/openai/whisper-large-v3-turbo`), HTTP client
  - `Transcribe(ctx, audio []byte, contentType string) (string, error)`: builds the REST request, sends, parses `result.text`, maps Cloudflare failures to clean errors (auth, rate/quota, audio-too-large, provider error)
  - Read config from env/settings the same way other integrations do (e.g. `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_API_TOKEN`), with a clean "not configured" error surfaced to the client
- No persistence of audio server-side; keep in-memory only, log nothing sensitive

### Phase 2 — HTTP route
- New route in `internal/routes/` registered alongside the others (see `routes.go`):
  - `POST /api/stt/transcribe` (auth-required like sibling routes)
  - Accept `multipart/form-data` field `audio` (file) — raw bytes forwarded to the service
  - Return JSON `{ "text": "...", "duration neurons": ... }` or error JSON matching existing route conventions
  - Cap upload size consistent with Cloudflare limits (~4 MB body for CPU models; if we ever exceed bound, chunk client-side — record & send in ≤30s segments)
- Wire the service into route construction/dependency injection following how `integrations` or `providers` are initialized

### Phase 3 — Desktop client (Rust) — optional this pass
- Add a command in console-core that records via the existing media capabilities if present; otherwise just expose the server endpoint to the composer as a "record → transcribe" attachment
- Keep minimal: a recording toggle + server call + insert transcript into composer draft

### Phase 4 — Android client (Kotlin) — optional this pass
- Same UX: record via `MediaRecorder` (AAC/M4A, mono OK) or AudioRecord → send to server endpoint → insert transcript
- Note: Client-side format conversion (mono 16 kHz) improves accuracy if economical; otherwise send native format, Cloudflare handles common containers

## Testing

- `apps/server-go/tests/stt/` with the actual transcription handler mocked upstream (httptest server replicating Cloudflare response shapes: success, quota exceeded, auth error)
- Run only the relevant test file: `cd apps/server-go && go test ./tests/stt/ -v`

## Costs & limits recap

| Item | Value |
|---|---|
| Free allocation | 10,000 neurons/day (all plans) |
| whisper-large-v3-turbo | 46.63 neurons/audio-min (~215 min/day free) |
| Paid beyond free | $0.011 / 1,000 neurons |
| Upload cap | Cloudflare Workers AI CPU model audio limit (~4 MB body) |
| Resets | Daily at 00:00 UTC |

## Open questions

1. Do we need streaming/live partial results? (If yes, revisit nova-3/flux — 10x cost.)
2. Any per-account teardown/refresh needed for token storage — follow existing integration-token pattern used by `internal/services/integrations/`.
