import { useMemo, useState, useCallback, useEffect } from "react";
import { useCreateSession, useDeleteSession, useProjects, useSessions, prefetchSession } from "./queries";
import { useQueryClient } from "@tanstack/react-query";
import type { SessionHeader } from "@console/types";
import { useAppStore, useChatStore } from "../stores";
import { useProjectBranches } from "./useProjectBranches";
import { folderName } from "../utils";
import { draftPreview, isDraftSession } from "../stores/chat/draft";

export interface GroupedProjectSection {
  projectId: string | null;
  projectName: string;
  data: SessionHeader[];
  /** Timestamp of the most-recent session in this section (for section sorting). */
  latestAt: number;
}

export function useHomeSessions() {
  const queryClient = useQueryClient();
  const { data: projects = [], refetch: refetchProjects } = useProjects();
  const { data: sessions = [], isLoading: isLoadingSessions, refetch: refetchSessions } = useSessions();
  const { data: branches = {} } = useProjectBranches(projects);
  const createSession = useCreateSession();
  const deleteSessionMutation = useDeleteSession();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
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

  // Draft map: per-session unsent input/attachments (max 2 images via draft.ts)
  // isDraftSession true for sessions with input.trim or attachments
  const draftSessions = useChatStore((s) => s.sessions);

  // Build DRAFT section for never-sent / unsent drafts (0 messages but has input)
  const draftSection = useMemo<GroupedProjectSection | null>(() => {
    const drafts: SessionHeader[] = [];
    for (const [id, state] of Object.entries(draftSessions)) {
      if (state.messages.length !== 0) continue;
      if (!isDraftSession(state)) continue;
      // Find server header for this id (if it was created via composeSession)
      const serverHeader = sessions.find((h) => h.id === id);
      if (serverHeader) {
        drafts.push({
          ...serverHeader,
          updatedAt: state.draftUpdatedAt ?? serverHeader.updatedAt,
        });
      } else {
        // Ephemeral local draft with no server session yet — synthesize
        // Use selected project as fallback cwd/projectId
        const fallbackProject = projects[0];
        drafts.push({
          id,
          title: draftPreview(state, 32),
          cwd: fallbackProject?.path ?? "",
          projectId: fallbackProject?.id,
          modelId: "",
          provider: "",
          createdAt: state.draftUpdatedAt ?? Date.now(),
          updatedAt: state.draftUpdatedAt ?? Date.now(),
          messageCount: 0,
          status: "idle" as const,
        });
      }
    }
    if (drafts.length === 0) return null;
    drafts.sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0));
    return {
      projectId: null,
      projectName: "DRAFT",
      data: drafts,
      latestAt: drafts[0]?.updatedAt ?? 0,
    };
  }, [draftSessions, sessions, projects]);

  // Group filtered sessions by project (by projectId or matching cwd path)
  const sections = useMemo<GroupedProjectSection[]>(() => {
    const byProject = new Map<
      string,
      { projectId: string | null; projectName: string; list: SessionHeader[] }
    >();

    // Exclude drafts with 0 messages from normal grouping — they live in DRAFT section
    const nonDraftSessions = filteredSessions.filter((s) => {
      const st = draftSessions[s.id];
      if (!st) return true;
      if (st.messages.length !== 0) return true;
      return !isDraftSession(st);
    });

    for (const session of nonDraftSessions) {
      const project =
        (session.projectId ? projects.find((p) => p.id === session.projectId) : undefined) ??
        projects.find((p) => p.path && session.cwd && p.path === session.cwd);

      const groupKey = project ? project.id : (folderName(session.cwd) || "draft").toLowerCase();
      const groupName = project ? project.name.toUpperCase() : (folderName(session.cwd) || "DRAFT").toUpperCase();

      const existing = byProject.get(groupKey);
      if (existing) {
        existing.list.push(session);
      } else {
        byProject.set(groupKey, {
          projectId: project?.id ?? session.projectId ?? null,
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
  }, [filteredSessions, projects, draftSessions, draftSection]);

  const openSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    setActiveTab("chat");
  };

  const composeSession = async (targetProjectId?: string | null) => {
    if (createSession.isPending) return;

    const project = targetProjectId
      ? projects.find((p) => p.id === targetProjectId)
      : projects[0];

    try {
      const session = await createSession.mutateAsync({
        cwd: project?.path ?? "",
        ...(project ? { projectId: project.id } : targetProjectId ? { projectId: targetProjectId } : {}),
        title: "New Chat",
      });
      setSelectedSessionId(session.id);
      setActiveTab("chat");
    } catch (error) {
      console.error("Failed to create session:", error);
      throw error;
    }
  };

  const getProjectForSession = (session: SessionHeader) => {
    return (
      (session.projectId ? projects.find((p) => p.id === session.projectId) : undefined) ??
      projects.find((p) => p.path && session.cwd && p.path === session.cwd)
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
        queryClient.invalidateQueries({ queryKey: ["sessions"] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
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
