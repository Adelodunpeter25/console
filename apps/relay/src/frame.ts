// Wire protocol between the relay and a console server (see README.md).
// Binary WebSocket messages: [type: u8][stream id: u32 big-endian][payload].

export const T = {
	REQ: 1, // json {method, path, headers}
	DATA: 2, // raw body bytes
	END: 3, // end of body
	RES: 4, // json {status, headers}
	WS_OPEN: 5, // json {path, headers}
	WS_TEXT: 6, // utf-8 text message
	WS_BIN: 7, // binary message
	CLOSE: 8, // json {code, reason}
	RESET: 9, // utf-8 error message; aborts the stream
} as const;

export type Headers2 = [string, string][];

const enc = new TextEncoder();
const dec = new TextDecoder();

export function encode(type: number, sid: number, payload?: Uint8Array | string): Uint8Array<ArrayBuffer> {
	const body = typeof payload === "string" ? enc.encode(payload) : (payload ?? new Uint8Array(0));
	const out = new Uint8Array(5 + body.length);
	out[0] = type;
	new DataView(out.buffer).setUint32(1, sid);
	out.set(body, 5);
	return out;
}

export function decode(buf: ArrayBuffer): { type: number; sid: number; payload: Uint8Array } | null {
	if (buf.byteLength < 5) return null;
	const view = new DataView(buf);
	return { type: view.getUint8(0), sid: view.getUint32(1), payload: new Uint8Array(buf, 5) };
}

export const json = (v: unknown) => JSON.stringify(v);
export const parse = <R>(p: Uint8Array): R => JSON.parse(dec.decode(p)) as R;
export const text = (p: Uint8Array) => dec.decode(p);
