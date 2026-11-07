// The shell test. Headless, this machine holds no stored device credential
// for the test backend URL, so the app boots onto the pairing card — the
// gate is the startup behaviour under test here. The sidebar's states are
// covered by the sidebar's own component tests.
import { expect, open, test } from "nsh/test";
import { Console } from "console-nsh/native";

test("an unpaired app boots onto the pairing card", async () => {
  const app = await open("./App.tsx");
  expect(app.texts()).toContain("Pair with Console server");
  expect(app.texts()).toContain("Pair");
});

test("Console.health rejects when the backend is unreachable", async () => {
  // Nothing listens here on purpose: the module must reject with the
  // reason, not resolve with a human string or hang.
  let message = "";
  try {
    await Console.health("http://localhost:59999");
  } catch (err) {
    message = err instanceof Error ? err.message : String(err);
  }
  expect(message).toContain("unreachable");
});
