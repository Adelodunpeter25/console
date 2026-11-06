// `nsh test` runs this file in a headless runner it builds with the crate.
import { expect, open, test } from "nsh/test";
import { Console } from "console-nsh/native";
import { groupSessions, groupSessionsByProject } from "./src/layout/session_groups";
import { formatTimeAgo } from "./src/utils/time";

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
  const groups = groupSessions(sessions, new Date(now));
  expect(groups.map((g) => g.label)).toEqual([
    "Pinned",
    "Today",
    "Yesterday",
    "Older",
  ]);
  expect(groups[1].sessions[0].title).toBe("Today's chat");
});

test("formats time ago from seconds or milliseconds", () => {
  const now = 1_700_000_000; // seconds
  expect(formatTimeAgo(now - 30, now)).toBe("just now");
  expect(formatTimeAgo((now - 300) * 1000, now)).toBe("5m");
  expect(formatTimeAgo(now - 7200, now)).toBe("2h");
  expect(formatTimeAgo(now - 3 * 86_400, now)).toBe("3d");
});

test("groups sessions by project, none last", () => {
  const sessions = [
    { id: "a", title: "A", status: "active", updatedAt: 3, projectId: "p1" },
    { id: "b", title: "B", status: "active", updatedAt: 2, projectId: null },
    { id: "c", title: "C", status: "active", updatedAt: 1, projectId: "p1" },
    {
      id: "d",
      title: "D",
      status: "active",
      updatedAt: 5,
      projectId: "p2",
    },
  ];
  const groups = groupSessionsByProject(sessions, (id) =>
    id === "p1" ? "Repo One" : id === "p2" ? "Repo Two" : null,
  );
  expect(groups.map((g) => g.label)).toEqual(["Repo One", "Repo Two", "None"]);
  expect(groups[0].sessions.map((s) => s.id)).toEqual(["a", "c"]);
});

test("Console.health resolves through console-core", async () => {
  // Nothing listens here on purpose: the module must resolve with an error
  // message, not reject or hang.
  const body = await Console.health("http://localhost:59999");
  expect(body).toContain("unreachable");
});
