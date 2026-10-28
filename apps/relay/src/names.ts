const ADJECTIVES = ["quiet", "brave", "swift", "calm", "bright", "lucky", "gentle", "bold", "sunny", "clever", "mellow", "witty", "cosmic", "amber", "misty", "nimble", "rapid", "silver", "golden", "cozy"];
const NOUNS = ["otter", "falcon", "panda", "maple", "comet", "harbor", "lantern", "meadow", "river", "summit", "willow", "badger", "heron", "pixel", "ember", "cedar", "orbit", "fox", "koala", "tiger"];

// Paths the relay itself uses, so they can never be a tunnel name.
const RESERVED = new Set(["register", "connect", "init", "admin", "api", "www", "health", "status", "favicon.ico", "robots.txt"]);

export const NAME_RE = /^[a-z0-9](?:[a-z0-9-]{1,30})[a-z0-9]$/; // 3-32 chars, no leading/trailing dash

export const validName = (n: string) => NAME_RE.test(n) && !n.includes("--") && !RESERVED.has(n);

const pick = <T>(a: T[]) => a[crypto.getRandomValues(new Uint32Array(1))[0]! % a.length]!;

export function generateName(): string {
	const n = crypto.getRandomValues(new Uint32Array(1))[0]! % 100;
	return `${pick(ADJECTIVES)}-${pick(NOUNS)}-${n}`;
}
