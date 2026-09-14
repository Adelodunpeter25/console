/**
 * Master Test Runner — Runs all offline service, storage, tool, system prompt, provider wire, command, todo, permission, and API tests.
 * Zero external LLM calls — 0 credits used.
 */
console.log("=========================================");
console.log("Starting Console Harness Offline Test Suite");
console.log("=========================================\n");

await import("./agent/agent-loop.test.js");
await import("./sessions/session-storage.test.js");
await import("./tools/tools.test.js");
await import("./tools/todo.test.js");
await import("./agent/system-prompt.test.js");
await import("./providers/providers-wire.test.js");
await import("./providers/opencode.test.js");
await import("./providers/discovery.test.js");
await import("./agent/permissions.test.js");
await import("./api/api.test.js");

console.log("=========================================");
console.log("🎉 ALL TESTS PASSED SUCCESSFULLY! (0 LLM credits consumed)");
console.log("=========================================");
export {};
