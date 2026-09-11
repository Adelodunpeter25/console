# Port Forwarding Client Integration API

## 1. Purpose

This document defines how Console clients integrate with the server-owned port-forwarding service.

The server owns port discovery, loopback probing, proxy-port allocation, and HTTP/WebSocket forwarding. Clients do not scan local sockets, infer proxy ports, or construct proxy URLs. Clients only:

1. Fetch the list of available forwarded ports.
2. Display each port using its real development port.
3. Open the returned `url` in a browser or WebView.
4. Refresh the list periodically.

The same API is used by the desktop client and the mobile client.

## 2. Base URL

All requests use the active Console environment's server URL.

Examples:

```text
http://localhost:3000/api/ports
https://console.example.com/api/ports
```

The client must use the existing environment/API transport configuration. It must not assume that the server is running on `localhost`.

## 3. Data Model

The public port object intentionally contains only the fields clients need:

```ts
export interface ForwardedPort {
  /** The development server's original port. */
  port: number;
  /** Fully-qualified URL to open in a browser or WebView. */
  url: string;
}
```

Example:

```json
{
  "port": 5173,
  "url": "http://192.168.1.10:45173/"
}
```

The server may retain ownership and lifecycle metadata internally, but fields such as process id, terminal id, job id, labels, status, and creation time are not part of the client API.

## 4. Common Response Envelope

Successful API responses use the standard Console envelope:

```ts
interface SuccessResponse<T> {
  success: true;
  data: T;
}
```

Errors use:

```ts
interface ErrorResponse {
  success: false;
  error: string;
}
```

Clients should check `success` before reading `data` and should show a recoverable error when a request fails.

## 5. List Forwarded Ports

### Request

```http
GET /api/ports
```

### Response

```json
{
  "success": true,
  "data": [
    { "port": 3000, "url": "http://192.168.1.10:45174/" },
    { "port": 5173, "url": "http://192.168.1.10:45173/" }
  ]
}
```

### Client behavior

- Treat every returned entry as available for preview.
- Display `port` as the user-facing label, for example `3000` or `5173`.
- Open `url` exactly as returned.
- Do not replace the host, port, scheme, or path in `url`.
- Do not construct a URL from the `port` field.
- An empty `data` array means that no previewable ports are currently available.

## 6. Manually Forward a Port

This endpoint is optional UI functionality for cases where automatic output detection misses a running development server.

### Request

```http
POST /api/ports/forward
Content-Type: application/json
```

```json
{
  "port": 8080
}
```

### Success response

```json
{
  "success": true,
  "data": {
    "port": 8080,
    "url": "http://192.168.1.10:45175/"
  }
}
```

### Failure response

If the server cannot connect to `127.0.0.1:8080`, it returns an error:

```json
{
  "success": false,
  "error": "Port 8080 is not listening on 127.0.0.1."
}
```

The client should validate that the port is an integer before sending the request and should display the server error without changing the current port list. A successful response should trigger an immediate list refresh.

## 7. Remove a Forwarded Port

### Request

```http
DELETE /api/ports/5173
```

### Response

```json
{
  "success": true,
  "data": {
    "port": 5173
  }
}
```

Deleting a forwarding entry revokes the preview URL. It does not terminate the development server. After deletion, the client should remove the entry locally or refresh the list.

A `404` means that the forwarding entry no longer exists. Clients should treat that as a successful local cleanup and refresh the list.

## 8. Refresh Strategy

Version one uses polling; there is no port-specific SSE subscription.

Recommended behavior:

- Fetch ports when the client starts or switches environment.
- Poll `GET /api/ports` every three seconds while the relevant screen is active.
- Stop or reduce polling when the client is suspended or the screen is not visible.
- Refresh immediately after manual forwarding or deletion.
- Replace the complete local list with each successful response.
- Keep the previous list during a transient request failure, and retry on the next interval.

The list response is the source of truth. Clients must not retain entries indefinitely after they disappear from a successful response.

## 9. Desktop Integration

### 9.1 Browser ports bar

The desktop client already has a native `BrowserView`. Add a port bar above its existing browser toolbar.

For each `ForwardedPort`:

```text
[ ● 3000 ] [ ● 5173 ] [ + ]
```

Behavior:

- Clicking a port opens that entry's `url` in the existing `BrowserView`.
- The chip text uses only `port`.
- The browser address bar displays the actual current `url`.
- The `+` action opens a manual port-forward form.
- A successful manual forward adds the returned entry and navigates to its `url` if appropriate.
- If navigation fails because the development server stopped after polling, show an error and refresh the list.

### 9.2 Browser navigation

The desktop browser must use the returned forwarding URL, not the original development URL.

Correct:

```text
http://192.168.1.10:45173/
```

Incorrect:

```text
http://localhost:5173/
```

The original port is only used for display and matching terminal output.

### 9.3 Terminal URL clicks

When terminal output contains a supported localhost URL, the desktop client should:

1. Extract the development port.
2. Find the matching `ForwardedPort` by `port`.
3. Switch to the Browser tab.
4. Navigate to the matching entry's `url`.

If no matching forwarded entry exists:

- Do not navigate to the raw localhost URL when the terminal belongs to a remote environment.
- Refresh the port list once.
- If it still does not exist, show that the port is not currently forwarded.

For a local environment where `localhost` is known to be valid, the client may offer the raw URL as a fallback, but this should be an explicit environment behavior rather than the default remote behavior.

## 10. Mobile Integration

The mobile client uses the same endpoints and response shape.

### Port list

- Fetch `GET /api/ports` when the preview screen opens.
- Poll while the preview screen is visible.
- Render one selectable row per returned port.
- Display the original `port` number.

### Opening a preview

When the user selects a port, open its returned `url`:

- Use an in-app WebView when the preview should remain inside Console.
- Use the system browser when the user chooses external preview.
- Preserve the URL exactly as returned.

The mobile client must not substitute the phone's `localhost`, the server API port, or the original development port.

There are no QR-code or LAN-specific client APIs in this version. Those can be added later without changing the basic `{ port, url }` contract.

## 11. Client State Shape

A minimal client state is sufficient:

```ts
interface PortForwardingState {
  ports: ForwardedPort[];
  loading: boolean;
  refreshing: boolean;
  error?: string;
  lastUpdated?: number;
}
```

The client does not need to model server-side ownership metadata. It only needs to know whether the list request succeeded and which URLs are currently available.

## 12. Transport Service Shape

The desktop and mobile transport layers should expose equivalent operations:

```ts
interface PortForwardingService {
  list(): Promise<ForwardedPort[]>;
  forward(port: number): Promise<ForwardedPort>;
  remove(port: number): Promise<void>;
}
```

For the Rust desktop client, the equivalent service should deserialize the same JSON response into a small `Port` type:

```rust
pub struct Port {
    pub port: u16,
    pub url: String,
}
```

The service should own HTTP details. UI code should receive typed ports rather than parse response envelopes or build endpoint URLs directly.

## 13. Error Handling

Clients should handle these cases:

| Situation | Expected client behavior |
|---|---|
| Server returns an empty list | Show an empty state; do not show an error |
| List request fails | Keep the previous list and show a non-blocking refresh error |
| Manual forward succeeds | Add/replace the entry and refresh |
| Manual forward returns `400` | Show the server error; keep existing entries |
| Delete returns `404` | Remove the entry locally and refresh |
| Preview URL returns `502` or connection failure | Show that the development server is no longer reachable and refresh |
| Environment changes | Clear the old list, stop the old poller, fetch the new environment |

## 14. Compatibility Rules

- The public entry shape is exactly `{ port, url }` for v1.
- Clients must ignore additional response fields if the server adds them later.
- Clients must not require labels, process ids, terminal ids, job ids, status values, or timestamps.
- Clients must not depend on the proxy-port allocation algorithm.
- Clients must not assume that the proxy URL uses the API server's port.
- Clients must use the active environment's configured base URL for API requests.
