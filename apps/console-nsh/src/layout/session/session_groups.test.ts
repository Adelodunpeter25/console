// Grouping tests, split out of the shell test (one file per component, the
// way the GPUI app's `tests/` mirror its sources).
import { expect, test } from "nsh/test";
import {
  groupSessions,
  groupSessionsByProject,
  partitionSettled,
  sentenceCaseProjectName,
} from "./session_groups";

test("groups sessions by pin and recency", () => {
  const now = 1_000_000_000_000;
  const day = 24 * 60 * 60 * 1000;
  const sessions = [
    { id: "a", title: "Pinned work", status: "active", pinned: true, settled: false, updatedAt: now },
    { id: "b", title: "Today's chat", status: "active", pinned: false, settled: false, updatedAt: now - 1000 },
    {
      id: "c",
      title: "Yesterday's chat",
      status: "active", pinned: false, settled: false,
      updatedAt: now - day,
    },
    {
      id: "d",
      title: "Old chat",
      status: "active", pinned: false, settled: false,
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

test("groups sessions by project, none last", () => {
  const sessions = [
    { id: "a", title: "A", status: "active", pinned: false, settled: false, updatedAt: 3, projectId: "p1" },
    { id: "b", title: "B", status: "active", pinned: false, settled: false, updatedAt: 2, projectId: null },
    { id: "c", title: "C", status: "active", pinned: false, settled: false, updatedAt: 1, projectId: "p1" },
    { id: "d", title: "D", status: "active", pinned: false, settled: false, updatedAt: 5, projectId: "p2" },
  ];
  const groups = groupSessionsByProject(sessions, (id) =>
    id === "p1" ? "Repo One" : id === "p2" ? "Repo Two" : null,
  );
  expect(groups.map((g) => g.label)).toEqual([
    "Repo Two",
    "Repo One",
    "No Project",
  ]);
  const repoOne = groups.find((g) => g.label === "Repo One");
  expect(repoOne?.sessions.map((s) => s.id)).toEqual(["a", "c"]);
});

test("folds unknown project ids into No Project", () => {
  const sessions = [
    { id: "a", title: "A", status: "active", pinned: false, settled: false, updatedAt: 3, projectId: "ghost" },
  ];
  const groups = groupSessionsByProject(sessions, () => null);
  expect(groups.map((g) => g.label)).toEqual(["No Project"]);
});

test("settled chats leave the active sections, newest first", () => {
  const base = { status: "active", pinned: false };
  const { active, settled } = partitionSettled([
    { id: "a", title: "A", updatedAt: 1, settled: false, ...base },
    { id: "b", title: "B", updatedAt: 2, settled: true, ...base },
    { id: "c", title: "C", updatedAt: 3, settled: true, ...base },
  ]);
  expect(active.map((s) => s.id)).toEqual(["a"]);
  expect(settled.map((s) => s.id)).toEqual(["c", "b"]);
});

test("project names show a capitalised first letter", () => {
  expect(sentenceCaseProjectName("console")).toBe("Console");
  expect(sentenceCaseProjectName("")).toBe("");
});
