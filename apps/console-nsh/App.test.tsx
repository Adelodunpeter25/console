// `nsh test` runs this file in a headless runner it builds with the crate.
import { expect, open, test } from "nsh/test";
import { Console } from "console-nsh/native";

test("renders the shell", async () => {
  const app = await open("./App.tsx");
  expect(app.texts()).toContain("Console");
  expect(app.texts()).toContain("Chat");
  expect(app.texts()).toContain("Files");
  expect(app.texts()).toContain("Terminal");
});

test("switches panes from the nav", async () => {
  const app = await open("./App.tsx");
  await app.press("Terminal");
  expect(app.texts()).toContain("Placeholder — panes arrive in phase 2.");
});

test("Console.health resolves through console-core", async () => {
  // Nothing listens here on purpose: the module must resolve with an error
  // message, not reject or hang.
  const body = await Console.health("http://localhost:59999");
  expect(body).toContain("unreachable");
});
