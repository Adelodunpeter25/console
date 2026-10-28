import { DurableObject } from "cloudflare:workers";
import { T, encode, decode, json, parse, text, type Headers2 } from "./frame.ts";

const CHUNK = 256 * 1024; // Workers cap websocket messages at 1 MiB
const HOP = new Set(["connection", "keep-alive", "transfer-encoding", "upgrade", "proxy-connection", "te", "trailer"]);
const NO_BODY = new Set([101, 204, 205, 304]);

interface Pending {
	resolve: (r: Response) => void;
	reject: (e: Error) => void;
	controller?: ReadableStreamDefaultController<Uint8Array>;
}

/** One instance per tunnel id: holds the console server's socket and proxies visitors over it. */
export class Tunnel extends DurableObject {
	private pending = new Map<number, Pending>();

	constructor(ctx: DurableObjectState, env: Cloudflare.Env) {
		super(ctx, env);
		this.ctx.setWebSocketAutoResponse(new WebSocketRequestResponsePair("ping", "pong"));
	}

	async fetch(request: Request): Promise<Response> {
		const url = new URL(request.url);

		if (url.pathname === "/init") {
			await this.ctx.storage.put("tokenHash", await request.text());
			return new Response("ok");
		}
		if (!(await this.ctx.storage.get("tokenHash"))) return new Response("unknown tunnel", { status: 404 });

		const parts = url.pathname.split("/"); // ["", route, id, ...rest]
		if (parts[1] === "connect") return this.acceptServer(request);

		const prefix = `/s/${parts[2]}`;
		const path = (url.pathname.slice(prefix.length) || "/") + url.search;
		return request.headers.get("upgrade")?.toLowerCase() === "websocket"
			? this.proxyWebSocket(request, path, prefix)
			: this.proxyHttp(request, path, prefix);
	}

	// ---- tunnel side (console server) ----

	private async acceptServer(request: Request): Promise<Response> {
		if (request.headers.get("upgrade") !== "websocket") return new Response("expected websocket", { status: 426 });
		const token = request.headers.get("authorization")?.replace(/^Bearer /, "") ?? "";
		const hash = await sha256(token);
		if (hash !== (await this.ctx.storage.get("tokenHash"))) return new Response("forbidden", { status: 403 });

		for (const old of this.ctx.getWebSockets("server")) old.close(1012, "replaced");
		const pair = new WebSocketPair();
		this.ctx.acceptWebSocket(pair[1], ["server"]);
		return new Response(null, { status: 101, webSocket: pair[0] });
	}

	private server(): WebSocket | undefined {
		return this.ctx.getWebSockets("server").find((w) => w.readyState === WebSocket.OPEN);
	}

	private onServerFrame(buf: ArrayBuffer) {
		const f = decode(buf);
		if (!f) return;
		const p = this.pending.get(f.sid);
		switch (f.type) {
			case T.RES: {
				if (!p) return;
				const { status, headers } = parse<{ status: number; headers: Headers2 }>(f.payload);
				const h = new Headers();
				for (const [k, v] of headers) if (!HOP.has(k.toLowerCase())) h.append(k, v);
				const body = NO_BODY.has(status)
					? null
					: new ReadableStream<Uint8Array>({
							start: (c) => void (p.controller = c),
							cancel: () => {
								this.pending.delete(f.sid);
								this.send(T.RESET, f.sid, "client gone");
							},
						});
				if (!body) this.pending.delete(f.sid);
				p.resolve(new Response(body, { status, headers: h }));
				return;
			}
			case T.DATA:
				p?.controller?.enqueue(f.payload.slice());
				return;
			case T.END:
				p?.controller?.close();
				this.pending.delete(f.sid);
				return;
			case T.RESET:
				if (p) {
					this.pending.delete(f.sid);
					if (p.controller) p.controller.error(new Error(text(f.payload)));
					else p.reject(new Error(text(f.payload)));
				}
				for (const c of this.ctx.getWebSockets(`c${f.sid}`)) c.close(1011, "upstream error");
				return;
			case T.WS_TEXT:
			case T.WS_BIN:
				for (const c of this.ctx.getWebSockets(`c${f.sid}`)) {
					c.send(f.type === T.WS_TEXT ? text(f.payload) : f.payload.slice());
				}
				return;
			case T.CLOSE: {
				const { code, reason } = parse<{ code: number; reason: string }>(f.payload);
				for (const c of this.ctx.getWebSockets(`c${f.sid}`)) c.close(safeCode(code), reason);
				return;
			}
		}
	}

	private send(type: number, sid: number, payload?: Uint8Array | string): boolean {
		const s = this.server();
		if (!s) return false;
		s.send(encode(type, sid, payload));
		return true;
	}

	private newSid(): number {
		for (;;) {
			const sid = crypto.getRandomValues(new Uint32Array(1))[0]!;
			if (!this.pending.has(sid) && this.ctx.getWebSockets(`c${sid}`).length === 0) return sid;
		}
	}

	// ---- visitor side ----

	private async proxyHttp(request: Request, path: string, prefix: string): Promise<Response> {
		if (!this.server()) return new Response("tunnel offline", { status: 502 });
		const sid = this.newSid();
		const done = new Promise<Response>((resolve, reject) => this.pending.set(sid, { resolve, reject }));

		this.send(T.REQ, sid, json({ method: request.method, path, headers: forwardHeaders(request, prefix) }));
		if (request.body) {
			const reader = request.body.getReader();
			try {
				for (;;) {
					const { value, done: end } = await reader.read();
					if (end) break;
					for (let i = 0; i < value.length; i += CHUNK) this.send(T.DATA, sid, value.subarray(i, i + CHUNK));
				}
			} catch {
				this.pending.delete(sid);
				this.send(T.RESET, sid, "request body aborted");
				return new Response("bad request", { status: 400 });
			}
		}
		this.send(T.END, sid);

		try {
			return await done;
		} catch (e) {
			return new Response(`tunnel error: ${(e as Error).message}`, { status: 502 });
		}
	}

	private proxyWebSocket(request: Request, path: string, prefix: string): Response {
		if (!this.server()) return new Response("tunnel offline", { status: 502 });
		const sid = this.newSid();
		const pair = new WebSocketPair();
		this.ctx.acceptWebSocket(pair[1], [`c${sid}`]);
		this.send(T.WS_OPEN, sid, json({ path, headers: forwardHeaders(request, prefix) }));
		return new Response(null, { status: 101, webSocket: pair[0] });
	}

	// ---- hibernatable websocket handlers ----

	async webSocketMessage(ws: WebSocket, message: string | ArrayBuffer) {
		const tag = this.ctx.getTags(ws)[0] ?? "";
		if (tag === "server") {
			if (typeof message !== "string") this.onServerFrame(message);
			return;
		}
		const sid = Number(tag.slice(1));
		if (typeof message === "string") this.send(T.WS_TEXT, sid, message);
		else this.send(T.WS_BIN, sid, new Uint8Array(message));
	}

	async webSocketClose(ws: WebSocket, code: number, reason: string) {
		this.onClosed(ws, code, reason);
	}

	async webSocketError(ws: WebSocket) {
		this.onClosed(ws, 1011, "error");
	}

	private onClosed(ws: WebSocket, code: number, reason: string) {
		const tag = this.ctx.getTags(ws)[0] ?? "";
		try {
			ws.close(safeCode(code), reason);
		} catch {}
		if (tag === "server") {
			if (this.server()) return; // a newer connection already took over
			for (const [sid, p] of this.pending) {
				if (p.controller) p.controller.error(new Error("tunnel closed"));
				else p.reject(new Error("tunnel closed"));
				this.pending.delete(sid);
			}
			for (const c of this.ctx.getWebSockets()) if (this.ctx.getTags(c)[0] !== "server") c.close(1012, "tunnel closed");
			return;
		}
		this.send(T.CLOSE, Number(tag.slice(1)), json({ code, reason }));
	}
}

function forwardHeaders(request: Request, prefix: string): Headers2 {
	const out: Headers2 = [];
	for (const [k, v] of request.headers) {
		const l = k.toLowerCase();
		if (HOP.has(l) || l.startsWith("cf-") || l.startsWith("sec-websocket-") || l === "host" || l.startsWith("x-forwarded-")) continue;
		out.push([k, v]);
	}
	out.push(["x-forwarded-prefix", prefix]);
	out.push(["x-forwarded-proto", "https"]);
	out.push(["x-forwarded-host", new URL(request.url).host]);
	return out;
}

// 1005/1006 may not be sent in a close frame.
const safeCode = (c: number) => (c === 1000 || (c >= 3000 && c < 5000) || (c >= 1001 && c <= 1014 && c !== 1004 && c !== 1005 && c !== 1006) ? c : 1000);

async function sha256(s: string): Promise<string> {
	const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(s));
	return Array.from(new Uint8Array(d), (b) => b.toString(16).padStart(2, "0")).join("");
}
