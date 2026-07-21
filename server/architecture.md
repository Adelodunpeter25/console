# Daemon Architecture

This document outlines the planned architecture for the headless TypeScript daemon.

## 1. Core Framework: Hono

- **Reasoning:** Chosen for its exceptional performance (high throughput, low latency) and extremely low memory usage, making it ideal for a long-running, efficient daemon.
- **Runtime:** The daemon will run on Node.js. Hono is runtime-agnostic, but we will target Node.js for its mature ecosystem.

## 2. Communication Protocols

The server will support two primary protocols to provide flexibility for client applications:

- **HTTP:** For stateless, request-response interactions. This is suitable for simple, one-off prompts.
- **WebSockets:** For persistent, real-time, bidirectional communication. This is ideal for interactive chat sessions, allowing for low-latency streaming of responses back to the client (e.g., your mobile app).

## 3. Core Logic: The `GeminiClient`

- **Inspiration:** The design is heavily inspired by the API client logic found in `oh-my-pi`.
- **Functionality:** A self-contained TypeScript class, `GeminiClient`, will be the heart of the application. It will be responsible for all direct communication with Google's APIs.
- **Responsibilities:**
    - Managing OAuth 2.0 credentials.
    - Automatically refreshing access tokens when they expire.
    - Constructing the correct request body and headers for the target provider.
    - Selecting the appropriate API endpoint based on the chosen provider.
    - Handling API responses and streaming them back.

## 4. Provider Model

The daemon will explicitly support two providers, accessible via distinct API routes:

- **`/gemini`:** Routes to the production Google Cloud Code Assist API (`cloudcode-pa.googleapis.com`).
- **`/antigravity`:** Routes to the sandbox/daily API (`daily-cloudcode-pa.googleapis.com`).

The `GeminiClient` will contain the internal logic to handle the specific request shaping required by each provider.

## 5. Authentication (OAuth 2.0)

- **Server-Side Handling:** The daemon will manage the entire OAuth 2.0 lifecycle on behalf of the client. The mobile app will not need to know anything about the tokens.
- **Configuration:** The server will be configured once with a long-lived `refreshToken`.
- **Process:** On startup, and whenever the `accessToken` expires, the `GeminiClient` will automatically use the `refreshToken` to request a new `accessToken` from Google's token endpoint. This ensures the service is always ready to make authenticated API calls.

## 6. Deployment & Dependencies

- **No Binaries:** The entire application will be pure TypeScript. It will have **no dependency** on external binaries like the ones used by `synara` or `oh-my-pi`.
- **Dependencies:** It will have a minimal set of dependencies, including `hono` and its WebSocket middleware.
- **Deployment:** The result will be a standard Node.js application that can be easily containerized (e.g., with Docker) or run directly on a server using a process manager like `pm2`.
