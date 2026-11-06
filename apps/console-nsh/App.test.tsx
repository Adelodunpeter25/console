// The shell test: the app mounts, its sections show, the sidebar's
// states resolve. Component tests live beside the components.
import { expect, open, test } from "nsh/test";
import { Console } from "console-nsh/native";

test("renders the shell's sections", async () => {
  const app = await open("./App.tsx");
  expect(app.texts()).toContain("PROJECTS");
  expect(app.texts()).toContain("CHATS");
  // Nothing listens on the backend URL in the runner: the sidebar shows its
  // empty state instead of hanging.
  expect(app.texts()).toContain("No projects yet");
});

test("Console.health resolves through console-core", async () => {
  // Nothing listens here on purpose: the module must resolve with an error
  // message, not reject or hang.
  const body = await Console.health("http://localhost:59999");
  expect(body).toContain("unreachable");
});
