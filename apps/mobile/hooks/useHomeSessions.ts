import { useMemo, useState } from "react";
import { useCreateSession, useProjects, useSessions } from "@console/api";
import type { SessionHeader } from "@console/types";
import { useAppStore } from "../stores";
import { useProjectBranches } from "./useProjectBranches";

export interface GroupedProjectSection {
  projectId: string | null;
  projectName: string;
  data: SessionHeader[];
  /** Timestamp of the most-recent session in this section (for section sorting). */
  latestAt: number;
}

function folderName(path?: string): string {
  if (!path) return "";
  const trimmed = path.replace(/\/+$/, "");
  return trimmed.slice(trimmed.lastIndexOf("/") + 1);
}

export function useHomeSessions() {
  const { data: projects = [] } = useProjects();
  const { data: sessions = [], isLoading: isLoadingSessions } = useSessions();
  const { data: branches = {} } = useProjectBranches(projects);
  const createSession = useCreateSession();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const [searchQuery, setSearchQuery] = useState("");

  // Filter sessions by search query
  const filteredSessions = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return sessions;
    return sessions.filter((s) => (s.title || "").toLowerCase().includes(q));
  }, [sessions, searchQuery]);

  // Group filtered sessions by project (by projectId or matching cwd path)
  const sections = useMemo<GroupedProjectSection[]>(() => {
    const byProject = new Map<
      string,
      { projectId: string | null; projectName: string; list: SessionHeader[] }
    >();

    for (const session of filteredSessions) {
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
    // Sort sections by their most-recent session (descending), so the latest
    // activity overall always sits at the top of the list.
    return result.sort((a, b) => b.latestAt - a.latestAt);
  }, [filteredSessions, projects]);

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

  return {
    sections,
    searchQuery,
    setSearchQuery,
    openSession,
    composeSession,
    isCreatingSession: createSession.isPending,
    isLoadingSessions,
    getProjectNameForSession,
    getBranchForSession,
    navigateToSettings: () => setActiveTab("settings"),
  };
}
