import assert from "node:assert/strict";
import { resolveAntigravityThinkingLevel } from "@/providers/src/antigravity/stream-fn.js";

console.log("Running Antigravity thinking-level tests...");

{
  const model = {
    supportedThinkingLevels: ["low", "medium", "high"] as const,
    defaultThinkingLevel: "medium" as const,
  };

  assert.equal(resolveAntigravityThinkingLevel(model), "medium");
  assert.equal(resolveAntigravityThinkingLevel(model, "high"), "high");
  assert.throws(() => resolveAntigravityThinkingLevel(model, "minimal"), /not supported/);
  assert.throws(() => resolveAntigravityThinkingLevel(model, "xhigh"), /not supported/);

  console.log("  ✅ defaults, overrides, and unsupported levels");
}

{
  assert.equal(
    resolveAntigravityThinkingLevel({ supportedThinkingLevels: ["minimal", "low"] }, "low"),
    "low",
  );
  assert.equal(resolveAntigravityThinkingLevel({}), undefined);
  console.log("  ✅ models without a configured default preserve current behavior");
}
