import { env } from "cloudflare:workers";
import { Tunnel } from "./tunnel.ts";
import { generateName, validName } from "./names.ts";

export { Tunnel };

async function sha256Hex(s: string): Promise<string> {
	const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(s));
	return Array.from(new Uint8Array(d), (b) => b.toString(16).padStart(2, "0")).join("");
}

const stub = (name: string, locationHint?: DurableObjectLocationHint) =>
	env.TUNNEL.get(env.TUNNEL.idFromName(name), locationHint ? { locationHint } : undefined);

// A tunnel's Durable Object is created once, wherever it is first used, and never
// moves. Hinting the caller's region at registration puts it near the console
// server, which is where every request has to end up.
function hintFor(cf: IncomingRequestCfProperties | undefined): DurableObjectLocationHint | undefined {
	const lon = Number(cf?.longitude ?? NaN);
	switch (cf?.continent) {
		case "AF": return "afr";
		case "SA": return "sam";
		case "OC": return "oc";
		case "AS": return cf.country && ["AE", "SA", "QA", "KW", "BH", "OM", "IL", "JO", "TR", "IQ", "IR"].includes(cf.country) ? "me" : "apac";
		case "EU": return lon > 20 ? "eeur" : "weur";
		case "NA": return lon < -100 ? "wnam" : "enam";
	}
	return undefined;
}

// Claims `name` for a new token. The Durable Object for a name is a single writer,
// so a 409 here means someone else already owns it.
async function claim(name: string, token: string, hint?: DurableObjectLocationHint): Promise<{ ok: boolean; colo?: string }> {
	const res = await stub(name, hint).fetch("https://do/init", { method: "POST", body: await sha256Hex(token) });
	return { ok: res.ok, colo: res.headers.get("x-relay-do") ?? undefined };
}

async function register(request: Request, origin: string): Promise<Response> {
	let want: string | undefined;
	try {
		const body = (await request.json()) as { name?: unknown };
		if (typeof body?.name === "string") want = body.name.trim().toLowerCase();
	} catch {} // empty body = generate a name

	const hint = hintFor((request as Request & { cf?: IncomingRequestCfProperties }).cf);
	const bytes = crypto.getRandomValues(new Uint8Array(32));
	const token = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, (c) => ({ "+": "-", "/": "_", "=": "" })[c]!);

	if (want !== undefined) {
		if (!validName(want)) return Response.json({ error: "invalid name: use 3-32 lowercase letters, digits or dashes" }, { status: 400 });
		const c = await claim(want, token, hint);
		if (!c.ok) return Response.json({ error: "name taken" }, { status: 409 });
		return Response.json({ name: want, token, url: `${origin}/${want}`, hint, colo: c.colo });
	}
	for (let i = 0; i < 8; i++) {
		const name = generateName();
		const c = await claim(name, token, hint);
		if (c.ok) return Response.json({ name, token, url: `${origin}/${name}`, hint, colo: c.colo });
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
			const res = await stub(first).fetch(request);
			// Where the request entered Cloudflare, to compare with x-relay-do.
			const cf = (request as Request & { cf?: IncomingRequestCfProperties }).cf;
			if (res.status === 101 || !cf?.colo) return res;
			const out = new Response(res.body, res);
			out.headers.set("x-relay-edge", String(cf.colo));
			return out;
		}
		return new Response("not found", { status: 404 });
	},
} satisfies ExportedHandler;
