import { useMemo, useState, useCallback, useEffect } from "react";
import {
  useCreateSession,
  useDeleteSession,
  useProjects,
  useSessions,
  prefetchSession,
  sessionKeys,
  fsKeys,
} from "./queries";
import { useQueryClient } from "@tanstack/react-query";
import type { SessionHeader } from "@console/types";
import { chat$ } from "@/stores/useChatStore";
import { useValue } from "@legendapp/state/react";
import { useProjectBranches } from "./useProjectBranches";
import { folderName } from "@/utils";
import { draftPreview, isDraftSession } from "@/stores/chat/draft";
import { openChatSession, setActiveTab, setSelectedSessionId } from "@/stores/useAppStore";

type DraftSummaries = Record<string, { preview: string; draftUpdatedAt?: number }>;

function draftSummariesEqual(a: DraftSummaries, b: DraftSummaries): boolean {
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);
  if (aKeys.length !== bKeys.length) return false;
  for (const k of aKeys) {
    const av = a[k];
    const bv = b[k];
    if (!bv) return false;
    if (av.preview !== bv.preview) return false;
    if (av.draftUpdatedAt !== bv.draftUpdatedAt) return false;
  }
  return true;
}

// Cached selector — returns the same DraftSummaries reference when the
// visible preview hasn't changed, so React's useSyncExternalStore sees a
// stable snapshot and doesn't warn "getSnapshot should be cached".
let cachedDrafts: DraftSummaries = {};
function selectDraftSummaries(): DraftSummaries {
  const next: DraftSummaries = {};
  for (const [id, state] of Object.entries(chat$.sessions.get())) {
    if (state.messages.length === 0 && isDraftSession(state)) {
      next[id] = {
        preview: draftPreview(state, 32),
        draftUpdatedAt: state.draftUpdatedAt,
      };
    }
  }
  if (draftSummariesEqual(cachedDrafts, next)) return cachedDrafts;
  cachedDrafts = next;
  return next;
}

export interface GroupedProjectSection {
  projectId: string | null;
  projectName: string;
  data: SessionHeader[];
  /** Timestamp of the most-recent session in this section (for section sorting). */
  latestAt: number;
}

function formatProjectTitle(name: string): string {
  if (!name) return "";
  return name
    .replace(/[-_]+/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim();
}

export function useHomeSessions() {
  const queryClient = useQueryClient();
  const { data: projects = [], refetch: refetchProjects } = useProjects();
  const { data: sessions = [], isLoading: isLoadingSessions, refetch: refetchSessions } = useSessions();
  const { data: branches = {} } = useProjectBranches(projects);
  const createSession = useCreateSession();
  const deleteSessionMutation = useDeleteSession();
  const [searchQuery, setSearchQuery] = useState("");
  const [isRefreshing, setIsRefreshing] = useState(false);

  // Background prefetch top 5 most recent sessions so opening any recent chat is instant
  useEffect(() => {
    if (sessions.length > 0) {
      const topSessions = sessions.slice(0, 5);
      const timer = setTimeout(() => {
        for (const s of topSessions) {
          prefetchSession(queryClient, s.id);
        }
      }, 300);
      return () => clearTimeout(timer);
    }
  }, [sessions, queryClient]);

  // Filter sessions by search query
  const filteredSessions = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return sessions;
    return sessions.filter((s) => (s.title || "").toLowerCase().includes(q));
  }, [sessions, searchQuery]);

  // Selective subscription: only track 0-message sessions with active drafts.
  // Returns primitives (preview text + timestamp) instead of full state objects
  // so Home doesn't re-render on unrelated draft state changes (running flag,
  // streaming, etc.) — only when the visible preview actually changes.
  // The selector is cached (returns same ref when equal) to satisfy
  // useSyncExternalStore's "getSnapshot should be cached" invariant.
  const draftSummaries = useValue(selectDraftSummaries);

  // Build DRAFT section for never-sent / unsent drafts (0 messages but has input)
  const draftSection = useMemo<GroupedProjectSection | null>(() => {
    const drafts: SessionHeader[] = [];
    for (const [id, summary] of Object.entries(draftSummaries)) {
      // Find server header for this id (if it was created via composeSession)
      const serverHeader = sessions.find((h) => h.id === id);
      if (serverHeader) {
        drafts.push({
          ...serverHeader,
          updatedAt: summary.draftUpdatedAt ?? serverHeader.updatedAt,
        });
      } else {
        // Ephemeral local draft with no server session yet — synthesize
        // Use selected project as fallback cwd/projectId
        const fallbackProject = projects[0];
        drafts.push({
          id,
          title: summary.preview,
          cwd: fallbackProject?.path ?? "",
          projectId: fallbackProject?.id,
          modelId: "",
          provider: "",
          createdAt: summary.draftUpdatedAt ?? Date.now(),
          updatedAt: summary.draftUpdatedAt ?? Date.now(),
          messageCount: 0,
          status: "idle" as const,
        });
      }
    }
    if (drafts.length === 0) return null;
    drafts.sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0));
    return {
      projectId: null,
      projectName: "Drafts",
      data: drafts,
      latestAt: drafts[0]?.updatedAt ?? 0,
    };
  }, [draftSummaries, sessions, projects]);

  // Group filtered sessions by project (by projectId or matching cwd path)
  const sections = useMemo<GroupedProjectSection[]>(() => {
    const byProject = new Map<
      string,
      { projectId: string | null; projectName: string; list: SessionHeader[] }
    >();

    // Exclude drafts with 0 messages from normal grouping — they live in DRAFT section
    // Also deduplicate any duplicate session ids returned by query
    const seenSessionIds = new Set<string>();
    const nonDraftSessions = filteredSessions.filter((s) => {
      if (s.id in draftSummaries || seenSessionIds.has(s.id)) return false;
      seenSessionIds.add(s.id);
      return true;
    });

    for (const session of nonDraftSessions) {
      // Scratchpad / General sessions: projectId == null (explicit scratch)
      if (session.projectId == null) {
        const groupKey = "general";
        const existing = byProject.get(groupKey);
        if (existing) {
          existing.list.push(session);
        } else {
          byProject.set(groupKey, {
            projectId: null,
            projectName: "General",
            list: [session],
          });
        }
        continue;
      }

      const project =
        projects.find((p) => p.path && session.cwd && p.path === session.cwd) ??
        (session.projectId ? projects.find((p) => p.id === session.projectId) : undefined);

      const resolvedProjectId = project?.id ?? session.projectId ?? null;
      const groupKey = resolvedProjectId
        ? `project-${resolvedProjectId}`
        : (folderName(session.cwd) || "draft").toLowerCase();
      const rawName = project ? project.name : (folderName(session.cwd) || "Drafts");
      const groupName = formatProjectTitle(rawName);

      const existing = byProject.get(groupKey);
      if (existing) {
        existing.list.push(session);
      } else {
        byProject.set(groupKey, {
          projectId: resolvedProjectId,
          projectName: groupName,
          list: [session],
        });
      }
    }

    const result: GroupedProjectSection[] = [];
    for (const [, group] of byProject) {
      const sorted = group.list.sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0));
      result.push({
        projectId: group.projectId,
        projectName: group.projectName,
        data: sorted,
        // Track the newest activity in this section so sections themselves
        // sort by recency — the project with the latest session comes first.
        latestAt: sorted[0]?.updatedAt ?? 0,
      });
    }
    const sortedRest = result.sort((a, b) => b.latestAt - a.latestAt);
    if (draftSection) return [draftSection, ...sortedRest];
    return sortedRest;
  }, [filteredSessions, projects, draftSummaries, draftSection]);

  const openSession = (sessionId: string) => {
    openChatSession(sessionId);
  };

  const composeSession = async (targetProjectId?: string | null) => {
    if (createSession.isPending) return;

    // Explicit null => scratchpad session (General)
    if (targetProjectId === null) {
      try {
        const session = await createSession.mutateAsync({
          projectId: null,
          title: "New Chat",
        });
        openChatSession(session.id);
      } catch (error) {
        console.error("Failed to create session:", error);
        throw error;
      }
      return;
    }

    const project = targetProjectId
      ? projects.find((p) => p.id === targetProjectId)
      : projects[0];

    try {
      const session = await createSession.mutateAsync({
        cwd: project?.path ?? "",
        ...(project ? { projectId: project.id } : targetProjectId ? { projectId: targetProjectId } : {}),
        title: "New Chat",
      });
      openChatSession(session.id);
    } catch (error) {
      console.error("Failed to create session:", error);
      throw error;
    }
  };

  const getProjectForSession = (session: SessionHeader) => {
    return (
      projects.find((p) => p.path && session.cwd && p.path === session.cwd) ??
      (session.projectId ? projects.find((p) => p.id === session.projectId) : undefined)
    );
  };

  const getProjectNameForSession = (session: SessionHeader) => {
    const project = getProjectForSession(session);
    return project?.name ?? folderName(session.cwd);
  };

  const getBranchForSession = (session: SessionHeader) => {
    return branches[session.projectId ?? ""];
  };

  const onRefresh = useCallback(async () => {
    setIsRefreshing(true);
    try {
      await Promise.all([
        refetchProjects(),
        refetchSessions(),
        queryClient.invalidateQueries({ queryKey: sessionKeys.all }),
        queryClient.invalidateQueries({ queryKey: fsKeys.projects }),
      ]);
    } finally {
      setIsRefreshing(false);
    }
  }, [refetchProjects, refetchSessions, queryClient]);

  return {
    sections,
    searchQuery,
    setSearchQuery,
    openSession,
    composeSession,
    deleteSession: (id: string) => deleteSessionMutation.mutateAsync(id),
    isCreatingSession: createSession.isPending,
    isLoadingSessions,
    isRefreshing,
    onRefresh,
    getProjectNameForSession,
    getBranchForSession,
    prefetchSession: (id: string) => prefetchSession(queryClient, id),
    navigateToSettings: () => setActiveTab("settings"),
  };
}
