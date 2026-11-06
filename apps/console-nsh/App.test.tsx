// `nsh test` runs this file in a headless runner it builds with the crate.
import { expect, open, test } from "nsh/test";
import { Console } from "console-nsh/native";
import { groupSessions } from "./src/layout/session_groups";

test("renders the shell's sections", async () => {
  const app = await open("./App.tsx");
  expect(app.texts()).toContain("PROJECTS");
  expect(app.texts()).toContain("CHATS");
  // Nothing listens on the backend URL in the runner: the sidebar shows its
  // empty state instead of hanging.
  expect(app.texts()).toContain("No projects yet");
});

test("groups sessions by pin and recency", () => {
  const now = 1_000_000_000_000;
  const day = 24 * 60 * 60 * 1000;
  const sessions = [
    { id: "a", title: "Pinned work", status: "pinned", updatedAt: now },
    { id: "b", title: "Today's chat", status: "active", updatedAt: now - 1000 },
    {
      id: "c",
      title: "Yesterday's chat",
      status: "active",
      updatedAt: now - day,
    },
    {
      id: "d",
      title: "Old chat",
      status: "active",
      updatedAt: now - 30 * day,
    },
  ];
  const groups = groupSessions(sessions, now);
  expect(groups.map((g) => g.label)).toEqual([
    "Pinned",
    "Today",
    "Yesterday",
    "Older",
  ]);
  expect(groups[1].sessions[0].title).toBe("Today's chat");
});

test("Console.health resolves through console-core", async () => {
  // Nothing listens here on purpose: the module must resolve with an error
  // message, not reject or hang.
  const body = await Console.health("http://localhost:59999");
  expect(body).toContain("unreachable");
});
