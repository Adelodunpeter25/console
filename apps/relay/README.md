# console relay

Cloudflare Worker + Durable Object that gives a console server a public URL on workers.dev
without opening ports: `https://console-relay.<account>.workers.dev/s/<id>/...`

## Flow
1. `POST /register` -> `{id, token, url}` (store these; the token is shown once, only its hash is kept).
2. The server opens an outbound WebSocket to `/connect/<id>` with `Authorization: Bearer <token>`.
3. Visitors hit `/s/<id>/<path>`; the Worker forwards them to that id's Durable Object, which
   sends them down the server's socket. The server replays them against its local port and answers.
   A new server connection replaces an older one. No connection -> 502.

The server must send `ping` text frames to keep idle sockets alive (answered with `pong` without waking the DO).

## Frames (binary WebSocket messages, `[type u8][stream id u32 BE][payload]`)
| type | name    | payload |
|------|---------|---------|
| 1 | REQ     | json `{method, path, headers:[[k,v]]}` (path includes query, prefix stripped) |
| 2 | DATA    | body bytes (<=256 KiB) |
| 3 | END     | end of body |
| 4 | RES     | json `{status, headers:[[k,v]]}`, then DATA..., END |
| 5 | WS_OPEN | json `{path, headers}`: a visitor opened a websocket |
| 6/7 | WS_TEXT / WS_BIN | websocket message, either direction |
| 8 | CLOSE   | json `{code, reason}`, either direction |
| 9 | RESET   | utf-8 error; aborts the stream |

Forwarded requests carry `x-forwarded-prefix: /s/<id>` so the server can build correct URLs.
The payloads are opaque to the relay, so an end-to-end encryption layer can be added later.

## Develop / deploy
    npm install
    npm run typecheck
    cf deploy
    node scripts/smoke.mjs https://console-relay.<account>.workers.dev   # end-to-end check

Note: `/register` is open to anyone; add a shared secret or rate limit before sharing the URL widely.
