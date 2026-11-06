// The shell test: the app mounts, its sections show, the sidebar's
// states resolve. Component tests live beside the components.
import { expect, open, test } from "nsh/test";
import { Console } from "console-nsh/native";

test("renders the shell's sections", async () => {
  const app = await open("./App.tsx");
  expect(app.texts()).toContain("PROJECTS");
  expect(app.texts()).toContain("CHATS");
  // Nothing listens on the backend URL in the runner: the sidebar shows
  // the load failure instead of hanging or pretending it is empty.
  expect(app.texts()).toContain("Couldn’t load projects");
});

test("Console.health rejects when the backend is unreachable", async () => {
  // Nothing listens here on purpose: the module must reject with the
  // reason, not resolve with a human string or hang.
  await expect(Console.health("http://localhost:59999")).rejects.toContain("unreachable");
});
