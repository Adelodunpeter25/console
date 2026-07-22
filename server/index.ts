/**
 * Server Entry Point — Starts Hono API Server listening on 0.0.0.0:3000.
 */
import { createServer } from "node:http";
import { createApiApp } from "./api/src/app.js";

const app = createApiApp();
const port = Number.parseInt(process.env.PORT || "3000", 10);
const host = process.env.HOST || "0.0.0.0";

const server = createServer(async (req, res) => {
  const url = `http://${req.headers.host || "localhost"}${req.url}`;

  // Convert Node.js IncomingMessage to Web Standard Request
  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (value !== undefined) {
      if (Array.isArray(value)) {
        for (const v of value) headers.append(key, v);
      } else {
        headers.set(key, value);
      }
    }
  }

  const method = req.method || "GET";
  let body: BodyInit | null = null;
  if (method !== "GET" && method !== "HEAD") {
    const chunks: Uint8Array[] = [];
    for await (const chunk of req) {
      chunks.push(chunk);
    }
    body = Buffer.concat(chunks);
  }

  const webReq = new Request(url, {
    method,
    headers,
    body,
  });

  const webRes = await app.fetch(webReq);

  res.statusCode = webRes.status;
  webRes.headers.forEach((val, key) => {
    res.setHeader(key, val);
  });

  if (webRes.body) {
    const reader = webRes.body.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      res.write(value);
    }
  }
  res.end();
});

server.listen(port, host, () => {
  console.log(`Console Agent Server running on http://${host}:${port}`);
  console.log(
    `API Base: http://${host}:${port}/api (Accepting connections from all hosts/devices)`,
  );
});
