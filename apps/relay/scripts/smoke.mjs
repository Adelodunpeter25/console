// End-to-end check against a deployed relay: node scripts/smoke.mjs https://console-relay.<acct>.workers.dev
// Acts as a tiny console server (http + websocket echo) and a visitor.
import http from "node:http";
import WebSocket, { WebSocketServer } from "ws";

const base = process.argv[2]?.replace(/\/$/, "");
if (!base) throw new Error("usage: smoke.mjs <relay-url>");

const local = http.createServer((req, res) => {
	let body = "";
	req.on("data", (c) => (body += c));
	req.on("end", () => {
		res.setHeader("content-type", "application/json");
		res.end(JSON.stringify({ method: req.method, url: req.url, body, prefix: req.headers["x-forwarded-prefix"] }));
	});
});
const wss = new WebSocketServer({ server: local });
wss.on("connection", (s) => s.on("message", (m, bin) => s.send(m, { binary: bin })));
await new Promise((r) => local.listen(0, r));
const port = local.address().port;

const reg = await (await fetch(`${base}/register`, { method: "POST", body: "{}" })).json();
reg.id = reg.name;
console.log("registered", reg.url);

const T = { REQ: 1, DATA: 2, END: 3, RES: 4, WS_OPEN: 5, WS_TEXT: 6, WS_BIN: 7, CLOSE: 8, RESET: 9 };
const frame = (type, sid, p = Buffer.alloc(0)) => {
	const b = Buffer.from(p);
	const out = Buffer.alloc(5 + b.length);
	out[0] = type; out.writeUInt32BE(sid, 1); b.copy(out, 5);
	return out;
};
const tunnel = new WebSocket(`${base.replace(/^http/, "ws")}/connect/${reg.id}`, { headers: { authorization: `Bearer ${reg.token}` } });
await new Promise((r, j) => { tunnel.on("open", r); tunnel.on("error", j); });
const streams = new Map(); // sid -> {req chunks | ws}
tunnel.on("message", (data) => {
	const buf = Buffer.from(data);
	const type = buf[0], sid = buf.readUInt32BE(1), payload = buf.subarray(5);
	if (type === T.REQ) {
		const { method, path, headers } = JSON.parse(payload);
		streams.set(sid, { method, path, headers: Object.fromEntries(headers), chunks: [] });
	} else if (type === T.DATA) streams.get(sid).chunks.push(payload);
	else if (type === T.END) {
		const s = streams.get(sid); streams.delete(sid);
		const r = http.request({ port, method: s.method, path: s.path, headers: { ...s.headers, host: `localhost:${port}` } }, (res) => {
			tunnel.send(frame(T.RES, sid, JSON.stringify({ status: res.statusCode, headers: Object.entries(res.headers).map(([k, v]) => [k, String(v)]) })));
			res.on("data", (c) => tunnel.send(frame(T.DATA, sid, c)));
			res.on("end", () => tunnel.send(frame(T.END, sid)));
		});
		r.end(Buffer.concat(s.chunks));
	} else if (type === T.WS_OPEN) {
		const { path } = JSON.parse(payload);
		const ws = new WebSocket(`ws://localhost:${port}${path}`);
		streams.set(sid, ws);
		ws.on("message", (m, bin) => tunnel.send(frame(bin ? T.WS_BIN : T.WS_TEXT, sid, m)));
		ws.on("close", (code, reason) => tunnel.send(frame(T.CLOSE, sid, JSON.stringify({ code, reason: reason.toString() }))));
	} else if (type === T.WS_TEXT || type === T.WS_BIN) {
		const ws = streams.get(sid);
		const send = () => ws.send(type === T.WS_TEXT ? payload.toString() : payload);
		ws.readyState === 1 ? send() : ws.once("open", send);
	} else if (type === T.CLOSE) streams.get(sid)?.close();
});

let fails = 0;
let named = null;
const check = (name, ok, extra = "") => { console.log(ok ? "PASS" : "FAIL", name, extra); if (!ok) fails++; };

let r = await fetch(`${reg.url}/api/hello?x=1`);
let j = await r.json();
check("GET path+query", r.status === 200 && j.url === "/api/hello?x=1" && j.prefix === `/${reg.name}`, JSON.stringify(j));

const big = "x".repeat(900_000);
r = await fetch(`${reg.url}/echo`, { method: "POST", body: big });
j = await r.json();
check("POST 900KB body", j.body.length === big.length && j.method === "POST");

const ws = new WebSocket(`${reg.url.replace(/^http/, "ws")}/socket`);
await new Promise((res, rej) => { ws.on("open", res); ws.on("error", rej); });
const echoed = new Promise((res) => ws.once("message", (m) => res(m.toString())));
ws.send("hello");
check("websocket echo", (await echoed) === "hello");
ws.close();

r = await fetch(`${base}/no-such-tunnel/x`);
check("unknown tunnel -> 404", r.status === 404);

const bad = new WebSocket(`${base.replace(/^http/, "ws")}/connect/${reg.id}`, { headers: { authorization: "Bearer nope" } });
const code = await new Promise((res) => { bad.on("unexpected-response", (_q, s) => res(s.statusCode)); bad.on("open", () => res(101)); bad.on("error", () => {}); });
check("bad token rejected", code === 403, String(code));

const mine = `smoke-${Math.random().toString(36).slice(2, 8)}`;
r = await fetch(`${base}/register`, { method: "POST", body: JSON.stringify({ name: mine }) });
j = await r.json();
check("custom name", r.status === 200 && j.url === `${base}/${mine}`, JSON.stringify(j));
r = await fetch(`${base}/register`, { method: "POST", body: JSON.stringify({ name: mine }) });
check("duplicate name -> 409", r.status === 409);
r = await fetch(`${base}/register`, { method: "POST", body: JSON.stringify({ name: "Bad Name!" }) });
check("invalid name -> 400", r.status === 400);
r = await fetch(`${base}/register`, { method: "POST", body: JSON.stringify({ name: "connect" }) });
check("reserved name -> 400", r.status === 400);
r = await fetch(`${reg.url}`, { redirect: "manual" });
check("no trailing slash redirects", r.status === 308);

tunnel.close();
await new Promise((r) => setTimeout(r, 1500));
r = await fetch(`${reg.url}/x`);
check("offline -> 502", r.status === 502, String(r.status));

console.log(fails ? `${fails} failed` : "all passed");
process.exit(fails ? 1 : 0);
