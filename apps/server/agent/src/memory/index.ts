export * from "./types.js";
export * from "./schema.js";
export * from "./storage.js";
export * from "./search.js";
export * from "./registry.js";

import { MemoryRegistry } from "./registry.js";

/** Process-wide singleton registry backing the real Console storage dir. */
export const memoryRegistry = new MemoryRegistry();
