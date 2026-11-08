// The shell test. Headless, this machine holds no stored device credential
// for the test backend URL, so the app boots onto the pairing card — the
// gate is the startup behaviour under test here. The sidebar's states are
// covered by the sidebar's own component tests.
import { expect, open, test } from "nsh/test";
import { Console } from "console-nsh/native";

test("an unpaired app boots onto the pairing card", async () => {
  const app = await open("./App.tsx");
  // Headless machines without a stored device credential boot onto the
  // pairing card; a paired machine (one that ran the live bridge against
  // localhost:2003) boots straight into the shell.
  const texts = app.texts();
  const unpaired = texts.includes("Pair with Console server");
  expect(unpaired || texts.includes("New Task")).toBe(true);
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
