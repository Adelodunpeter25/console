import React, { useCallback, useEffect, useMemo, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, BackHandler, ScrollView } from "react-native";
import { LegendList } from "@legendapp/list/react-native";
import { ChevronDown, ChevronRight, RefreshCw } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { FileIcon } from "@/components/icons/file-icon";
import { DiffView } from "@/components/chat/tools/diff-view";
import { useSessionChanges } from "@/hooks/queries";
import { useGitStatus } from "@/hooks/useGit";
import { sessionsView$ } from "@/stores/useSessionStore";
import { app$, setActiveTab } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";
import { project$ } from "@/stores/useProjectStore";
import { gitService } from "@console/api";
import { computeLineDiff } from "@/utils/diff";
import type { SessionFileChange } from "@console/types";
import { theme } from "@/styles/theme";

type Scope = "turn" | "all";
type Row =
  | { kind: "folder"; key: string; name: string; additions: number; deletions: number; count: number }
  | { kind: "file"; key: string; path: string; name: string; rel: string; status: string; additions: number; deletions: number };

function statusLetter(s: string): string {
  const u = s.toUpperCase();
  if (u.startsWith("ADD") || u === "A") return "A";
  if (u.startsWith("DEL") || u === "D") return "D";
  if (u === "R") return "R";
  return "M";
}

function statusColor(s: string): string {
  const l = statusLetter(s);
  if (l === "A") return "#34d399";
  if (l === "D") return "#f87171";
  if (l === "R") return "#38bdf8";
  return "#facc15";
}

function dirOf(path: string): string {
  const i = path.lastIndexOf("/");
  return i > 0 ? path.slice(0, i) : "";
}

function baseOf(path: string): string {
  const i = path.lastIndexOf("/");
  return i >= 0 ? path.slice(i + 1) : path;
}

function parseUnifiedDiff(diff: string) {
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

export function ChangesScreen() {
  const selectedSessionId = useValue(app$.selectedSessionId);
  const previousTab = useValue(app$.previousTab);
  const projects = useValue(project$.projects);
  const sessionCwd = useValue(() =>
    selectedSessionId ? sessionsView$[selectedSessionId].sessionCwd.get() ?? null : null,
  );
  const goBack = useCallback(() => {
    setActiveTab(previousTab && previousTab !== "changes" ? previousTab : "chat");
  }, [previousTab]);

  const project = useMemo(
    () => projects.find((p) => (sessionCwd ? p.path === sessionCwd || sessionCwd.startsWith(p.path + "/") : false)) ?? projects[0] ?? null,
    [projects, sessionCwd],
  );
  const repoPath = sessionCwd ?? project?.path ?? null;

  const { data: sessionChanges = [], isLoading, error, refetch, isFetching } = useSessionChanges(selectedSessionId ?? "");
  const { summary: gitSummary, refetch: refetchGit } = useGitStatus(repoPath);

  const [scope, setScope] = useState<Scope>("all");
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [diffText, setDiffText] = useState<string | null>(null);
  const [diffLoading, setDiffLoading] = useState(false);

  const scoped: SessionFileChange[] = useMemo(() => {
    if (scope === "all" || sessionChanges.length === 0) return sessionChanges;
    const max = Math.max(...sessionChanges.map((c) => c.turnIndex ?? 0));
    return sessionChanges.filter((c) => (c.turnIndex ?? 0) === max);
  }, [sessionChanges, scope]);

  const totals = useMemo(() => ({
    files: scoped.length,
    additions: scoped.reduce((a, c) => a + (c.additions ?? 0), 0),
    deletions: scoped.reduce((a, c) => a + (c.deletions ?? 0), 0),
  }), [scoped]);

  const rows = useMemo<Row[]>(() => {
    const groups = new Map<string, SessionFileChange[]>();
    for (const c of scoped) {
      const d = dirOf(c.path) || ".";
      const g = groups.get(d) ?? [];
      g.push(c);
      groups.set(d, g);
    }
    const out: Row[] = [];
    const sorted = [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]));
    for (const [dir, files] of sorted) {
      const isCollapsed = collapsed.has(dir);
      out.push({
        kind: "folder", key: `dir:${dir}`, name: dir,
        additions: files.reduce((a, f) => a + (f.additions ?? 0), 0),
        deletions: files.reduce((a, f) => a + (f.deletions ?? 0), 0),
        count: files.length,
      });
      if (isCollapsed) continue;
      const sortedFiles = [...files].sort((a, b) => a.path.localeCompare(b.path));
      for (const f of sortedFiles) {
        out.push({
          kind: "file", key: `file:${f.path}`, path: f.path, name: baseOf(f.path),
          rel: repoPath ? f.path.replace(repoPath + "/", "") : f.path,
          status: f.status, additions: f.additions ?? 0, deletions: f.deletions ?? 0,
        });
      }
    }
    return out;
  }, [scoped, collapsed, repoPath]);

  const toggleFolder = useCallback((name: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  useEffect(() => {
    if (!selectedPath || !repoPath) return;
    let cancelled = false;
    setDiffLoading(true);
    setDiffText(null);
    gitService.getDiff(repoPath, selectedPath).then((d) => {
      if (!cancelled) { setDiffText(d); setDiffLoading(false); }
    }).catch(() => { if (!cancelled) setDiffLoading(false); });
    return () => { cancelled = true; };
  }, [selectedPath, repoPath]);

  useEffect(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      if (selectedPath) { setSelectedPath(null); return true; }
      goBack();
      return true;
    });
    return () => sub.remove();
  }, [selectedPath, goBack]);

  const selectedChange = selectedPath ? scoped.find((c) => c.path === selectedPath) ?? null : null;
  const selectedRel = selectedPath ? (repoPath ? selectedPath.replace(repoPath + "/", "") : selectedPath) : "";
  const diff = useMemo(() => {
    if (!diffText) return null;
    try { return parseUnifiedDiff(diffText); } catch { return null; }
  }, [diffText]);
  const fallbackDiff = useMemo(() => {
    if (diff || !selectedChange) return null;
    if (selectedChange.status === "added") return { lines: [], addedCount: selectedChange.additions ?? 0, removedCount: 0 };
    return null;
  }, [diff, selectedChange]);

  if (selectedPath) {
    return (
      <View className="flex-1 bg-screen">
        <ScreenHeader title={baseOf(selectedPath)} subtitle={selectedRel} onBack={() => setSelectedPath(null)} />
        <ScrollView className="flex-1" contentContainerStyle={{ padding: 12, paddingBottom: 40 }}>
          {selectedChange ? (
            <View className="flex-row items-center gap-2 mb-3">
              <Text style={{ color: statusColor(selectedChange.status) }} className="text-xs font-bold">{statusLetter(selectedChange.status)}</Text>
              <Text className="text-xs font-mono text-emerald-400">+{selectedChange.additions ?? 0}</Text>
              <Text className="text-xs font-mono text-red-400">-{selectedChange.deletions ?? 0}</Text>
            </View>
          ) : null}
          {diffLoading ? (
            <View className="items-center py-10"><ActivityIndicator color={theme.colors.text.muted} /></View>
          ) : diff ? (
            <DiffView diff={diff} filePath={selectedPath} />
          ) : fallbackDiff ? (
            <View className="rounded-xl bg-black/40 p-3"><Text className="text-xs font-mono text-foreground-secondary">New file +{fallbackDiff.addedCount} lines (diff unavailable off-git).</Text></View>
          ) : (
            <View className="rounded-xl bg-black/40 p-3"><Text className="text-xs font-mono text-foreground-secondary">No diff available for this file.</Text></View>
          )}
        </ScrollView>
      </View>
    );
  }

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader
        title={gitSummary?.branch ? gitSummary.branch : "Changes"}
        subtitle={`${totals.files} files  +${totals.additions} -${totals.deletions}`}
        onBack={goBack}
        rightAction={
          <Pressable onPress={() => { refetch(); refetchGit(); }} className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center" style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}>
            <RefreshCw size={16} color={theme.colors.text.secondary} />
          </Pressable>
        }
      />
      <View className="flex-row px-4 pb-2 gap-2">
        {( (["turn", "all"] as Scope[]).map((s) => (
          <Pressable key={s} onPress={() => setScope(s)} className={`px-3 py-1.5 rounded-full border ${scope === s ? "bg-foreground border-foreground" : "bg-card border-border"}`}>
            <Text className={`text-xs font-semibold ${scope === s ? "text-black" : "text-foreground-secondary"}`}>{s === "turn" ? "This Turn" : "All Turns"}</Text>
          </Pressable>
        )) )}
      </View>
      {isLoading ? (
        <View className="flex-1 items-center justify-center"><ActivityIndicator color={theme.colors.text.muted} /><Text className="mt-2 text-xs text-foreground-muted">Loading changes…</Text></View>
      ) : error ? (
        <View className="flex-1 items-center justify-center px-6"><Text className="text-sm font-bold text-foreground">Couldn&apos;t load changes</Text><Text className="mt-1 text-xs text-foreground-muted">{(error as Error).message}</Text></View>
      ) : rows.length === 0 ? (
        <View className="flex-1 items-center justify-center px-6"><Text className="text-sm font-bold text-foreground">No working tree changes</Text><Text className="mt-1 text-xs text-foreground-muted text-center">Edits from this session will appear here.</Text></View>
      ) : (
        <LegendList
          data={rows}
          keyExtractor={(item) => item.key}
          recycleItems={false}
          estimatedItemSize={44}
          contentContainerStyle={{ paddingBottom: 24 }}
          renderItem={({ item }) => {
            if (item.kind === "folder") {
              const isCollapsed = collapsed.has(item.name);
              return (
                <Pressable onPress={() => toggleFolder(item.name)} className="flex-row items-center px-4 py-2 gap-1.5">
                  {isCollapsed ? <ChevronRight size={14} color={theme.colors.text.muted} /> : <ChevronDown size={14} color={theme.colors.text.muted} />}
                  <Text className="text-xs font-semibold text-foreground-secondary flex-1" numberOfLines={1}>{item.name} · {item.count}</Text>
                  <Text className="text-[11px] font-mono text-emerald-400">+{item.additions}</Text>
                  <Text className="text-[11px] font-mono text-red-400">-{item.deletions}</Text>
                </Pressable>
              );
            }
            return (
              <Pressable onPress={() => setSelectedPath(item.path)} className="flex-row items-center px-4 py-2 gap-2 active:opacity-70">
                <Text style={{ color: statusColor(item.status) }} className="text-[11px] font-bold w-3 text-center">{statusLetter(item.status)}</Text>
                <FileIcon filename={item.name} size={16} />
                <View className="flex-1 min-w-0">
                  <Text className="text-[13px] text-foreground" numberOfLines={1}>{item.name}</Text>
                  <Text className="text-[11px] text-foreground-secondary" numberOfLines={1}>{item.rel}</Text>
                </View>
                <Text className="text-[11px] font-mono text-emerald-400">+{item.additions}</Text>
                <Text className="text-[11px] font-mono text-red-400">-{item.deletions}</Text>
                <ChevronRight size={14} color={theme.colors.text.muted} />
              </Pressable>
            );
          }}
        />
      )}
      {isFetching ? <View className="absolute top-1 right-4"><ActivityIndicator size="small" color={theme.colors.text.muted} /></View> : null}
    </View>
  );
}

export default ChangesScreen;
