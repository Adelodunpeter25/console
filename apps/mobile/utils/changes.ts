import type { SessionFileChange } from "@console/types";

export type ChangesScope = "turn" | "all";

export type ChangesRow =
  | { kind: "folder"; key: string; name: string; additions: number; deletions: number; count: number }
  | { kind: "file"; key: string; path: string; name: string; rel: string; status: string; additions: number; deletions: number };

export function statusLetter(s: string): string {
  const u = (s ?? "").toUpperCase();
  if (u.startsWith("ADD") || u === "A") return "A";
  if (u.startsWith("DEL") || u === "D") return "D";
  if (u === "R") return "R";
  return "M";
}

export function statusColor(s: string): string {
  const l = statusLetter(s);
  if (l === "A") return "#34d399";
  if (l === "D") return "#f87171";
  if (l === "R") return "#38bdf8";
  return "#facc15";
}

export function dirOf(path: string): string {
  const i = path.lastIndexOf("/");
  return i > 0 ? path.slice(0, i) : "";
}

export function baseOf(path: string): string {
  const i = path.lastIndexOf("/");
  return i >= 0 ? path.slice(i + 1) : path;
}

export function stripRepoPrefix(path: string, repoPath: string | null): string {
  if (repoPath && path.startsWith(repoPath + "/")) return path.slice(repoPath.length + 1);
  return path;
}

export function filterByScope(changes: SessionFileChange[], scope: ChangesScope): SessionFileChange[] {
  if (scope === "all" || changes.length === 0) return changes;
  let max = 0;
  for (const c of changes) max = Math.max(max, c.turnIndex ?? 0);
  return changes.filter((c) => (c.turnIndex ?? 0) === max);
}

export function sumTotals(changes: SessionFileChange[]): { files: number; additions: number; deletions: number } {
  let additions = 0;
  let deletions = 0;
  for (const c of changes) {
    additions += c.additions ?? 0;
    deletions += c.deletions ?? 0;
  }
  return { files: changes.length, additions, deletions };
}

export function buildRows(scoped: SessionFileChange[], collapsed: ReadonlySet<string>, repoPath: string | null): ChangesRow[] {
  const groups = new Map<string, SessionFileChange[]>();
  for (const c of scoped) {
    const d = dirOf(c.path) || ".";
    const g = groups.get(d);
    if (g) g.push(c);
    else groups.set(d, [c]);
  }
  const out: ChangesRow[] = [];
  const sortedDirs = [...groups.keys()].sort((a, b) => a.localeCompare(b));
  for (const dir of sortedDirs) {
    const files = groups.get(dir)!;
    let additions = 0;
    let deletions = 0;
    for (const f of files) {
      additions += f.additions ?? 0;
      deletions += f.deletions ?? 0;
    }
    out.push({ kind: "folder", key: `dir:${dir}`, name: dir, additions, deletions, count: files.length });
    if (collapsed.has(dir)) continue;
    const sortedFiles = [...files].sort((a, b) => a.path.localeCompare(b.path));
    for (const f of sortedFiles) {
      out.push({
        kind: "file", key: `file:${f.path}`, path: f.path, name: baseOf(f.path),
        rel: stripRepoPrefix(f.path, repoPath),
        status: f.status, additions: f.additions ?? 0, deletions: f.deletions ?? 0,
      });
    }
  }
  return out;
}

export function parseUnifiedDiff(diff: string): { lines: { type: "added" | "removed" | "context"; text: string }[]; addedCount: number; removedCount: number } {
  const lines = diff.split("\n");
  const out: { type: "added" | "removed" | "context"; text: string }[] = [];
  let added = 0;
  let removed = 0;
  for (const l of lines) {
    if (l.startsWith("+++") || l.startsWith("---") || l.startsWith("@@") || l.startsWith("diff ") || l.startsWith("index ")) continue;
    if (l.startsWith("+")) { out.push({ type: "added", text: l.slice(1) }); added++; }
    else if (l.startsWith("-")) { out.push({ type: "removed", text: l.slice(1) }); removed++; }
    else out.push({ type: "context", text: l.startsWith(" ") ? l.slice(1) : l });
  }
  return { lines: out, addedCount: added, removedCount: removed };
}
