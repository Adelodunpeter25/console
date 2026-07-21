#!/usr/bin/env tsx
/**
 * Simple test script for the OAuth login flow
 */

import { loginGemini, loginAntigravity } from "./src/index.js";

const args = process.argv.slice(2);

if (args.length === 0 || args[0] === "--antigravity") {
  console.log("Starting Antigravity login...");
  await loginAntigravity();
} else if (args[0] === "--gemini") {
  console.log("Starting Gemini login...");
  await loginGemini();
} else {
  console.log("Usage:");
  console.log("  tsx login-test.ts --antigravity - Login to Antigravity (default)");
  console.log("  tsx login-test.ts --gemini      - Login to Gemini CLI");
}
