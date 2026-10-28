import { env } from "cloudflare:workers";
import { Tunnel } from "./tunnel.ts";

export { Tunnel };

const ALPHABET = "abcdefghijklmnopqrstuvwxyz23456789";

function randomId(n: number): string {
	const bytes = crypto.getRandomValues(new Uint8Array(n));
	return Array.from(bytes, (b) => ALPHABET[b % ALPHABET.length]).join("");
}

async function sha256Hex(s: string): Promise<string> {
	const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(s));
	return Array.from(new Uint8Array(d), (b) => b.toString(16).padStart(2, "0")).join("");
}

const stub = (id: string) => env.TUNNEL.get(env.TUNNEL.idFromName(id));

export default {
	async fetch(request: Request): Promise<Response> {
		const url = new URL(request.url);
		const [, route, id] = url.pathname.split("/");

		// POST /register -> {id, token, url}
		if (url.pathname === "/register" && request.method === "POST") {
			const tunnelId = randomId(16);
			const bytes = crypto.getRandomValues(new Uint8Array(32));
			const token = btoa(String.fromCharCode(...bytes)).replace(/[+/=]/g, (c) => ({ "+": "-", "/": "_", "=": "" })[c]!);
			const res = await stub(tunnelId).fetch("https://do/init", {
				method: "POST",
				body: await sha256Hex(token),
			});
			if (!res.ok) return new Response("register failed", { status: 500 });
			return Response.json({ id: tunnelId, token, url: `${url.origin}/s/${tunnelId}` });
		}

		// GET /connect/<id> (websocket, Authorization: Bearer <token>) and /s/<id>/...
		if ((route === "connect" || route === "s") && /^[a-z0-9]{16}$/.test(id ?? "")) {
			return stub(id!).fetch(request);
		}

		if (url.pathname === "/") return new Response("console relay\n");
		return new Response("not found", { status: 404 });
	},
} satisfies ExportedHandler;
