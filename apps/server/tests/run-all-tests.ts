/**
 * Master Test Runner — Runs all offline service, storage, tool, system prompt, provider wire, command, todo, permission, and API tests.
 * Zero external LLM calls — 0 credits used.
 */
console.log("=========================================");
console.log("Starting Console Harness Offline Test Suite");
console.log("=========================================\n");

await import("./agent-loop.test.js");
await import("./session-storage.test.js");
await import("./tools.test.js");
await import("./todo.test.js");
await import("./system-prompt.test.js");
await import("./providers-wire.test.js");
await import("./opencode.test.js");
await import("./discovery.test.js");
await import("./permissions.test.js");
await import("./api.test.js");

console.log("=========================================");
console.log("🎉 ALL TESTS PASSED SUCCESSFULLY! (0 LLM credits consumed)");
console.log("=========================================");
export {};
