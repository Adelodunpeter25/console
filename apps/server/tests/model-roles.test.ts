import assert from "node:assert/strict";
import { fallbackSessionTitle, isGenericSessionTitle, sanitizeSessionTitle } from "@/agent/src/service/session-title.js";

console.log("Running model roles tests...");

assert.equal(isGenericSessionTitle("New Session"), true);
assert.equal(isGenericSessionTitle("My custom title"), false);
assert.equal(fallbackSessionTitle("  Build   the login flow for users  "), "Build the login flow for users");
assert.equal(sanitizeSessionTitle('"Build the login flow\nfor users."'), "Build the login flow for users");
assert.equal(sanitizeSessionTitle("# Fix the broken authentication middleware now"), "Fix the broken authentication middleware now");

console.log("  ✅ Session title fallback and sanitization");
console.log("Model roles tests passed!\n");
