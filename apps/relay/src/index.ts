import { env } from "cloudflare:workers";
import { Tunnel } from "./tunnel.ts";
import { generateName, validName } from "./names.ts";

export { Tunnel };

async function sha256Hex(s: string): Promise<string> {
	const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(s));
	return Array.from(new Uint8Array(d), (b) => b.toString(16).padStart(2, "0")).join("");
}

const stub = (name: string) => env.TUNNEL.get(env.TUNNEL.idFromName(name));

// Claims `name` for a new token. The Durable Object for a name is a single writer,
// so a 409 here means someone else already owns it.
async function claim(name: string, token: string): Promise<boolean> {
	const res = await stub(name).fetch("https://do/init", { method: "POST", body: await sha256Hex(token) });
	return res.ok;
}

async function register(request: Request, origin: string): Promise<Response> {
	let want: string | undefined;
	try {
		const body = (await request.json()) as { name?: unknown };
		if (typeof body?.name === "string") want = body.name.trim().toLowerCase();
	} catch {} // empty body = generate a name

	const bytes = crypto.getRandomValues(new Uint8Array(32));
	const token = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, (c) => ({ "+": "-", "/": "_", "=": "" })[c]!);

	if (want !== undefined) {
		if (!validName(want)) return Response.json({ error: "invalid name: use 3-32 lowercase letters, digits or dashes" }, { status: 400 });
		if (!(await claim(want, token))) return Response.json({ error: "name taken" }, { status: 409 });
		return Response.json({ name: want, token, url: `${origin}/${want}` });
	}
	for (let i = 0; i < 8; i++) {
		const name = generateName();
		if (await claim(name, token)) return Response.json({ name, token, url: `${origin}/${name}` });
	}
	return Response.json({ error: "could not allocate a name, retry" }, { status: 503 });
}

export default {
	async fetch(request: Request): Promise<Response> {
		const url = new URL(request.url);
		const [, first = "", second = ""] = url.pathname.split("/");

		if (url.pathname === "/register" && request.method === "POST") return register(request, url.origin);

		// GET /connect/<name>: the console server's outbound websocket.
		if (first === "connect") return validName(second) ? stub(second).fetch(request) : new Response("not found", { status: 404 });

		if (url.pathname === "/") return new Response("console relay\n");

		// /<name>/...: a visitor.
		if (validName(first)) {
			if (url.pathname === `/${first}`) return Response.redirect(`${url.origin}/${first}/${url.search}`, 308);
			return stub(first).fetch(request);
		}
		return new Response("not found", { status: 404 });
	},
} satisfies ExportedHandler;
