import React from "react";
import { Folder, Home, MessageSquare, Settings, ChevronLeft, Plus } from "lucide-react";
import type { SessionHeader } from "@console/types";
import { useAppStore, useProjectStore } from "../store";
import { formatRelativeTime } from "../utils/time";

export function Sidebar() {
  const {
    activeView,
    setActiveView,
    selectedProjectId,
    setSelectedProjectId,
    setSelectedSessionId,
    sidebarOpen,
    toggleSidebar,
  } = useAppStore();

  const {
    projects,
    loading,
    loadProjects,
    sessionsByProject,
    loadSessions,
    createSession,
  } = useProjectStore();

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  const handleProjectClick = (projectId: string) => {
    if (selectedProjectId === projectId) {
      setSelectedProjectId(null);
      setSelectedSessionId(null);
    } else {
      setSelectedProjectId(projectId);
      setSelectedSessionId(null);
      loadSessions(projectId);
    }
  };

  const handleNewChat = async (projectId: string, cwd: string) => {
    const session = await createSession(cwd, projectId, "New Chat");
    setSelectedSessionId(session.id);
    setActiveView("chat");
  };

  if (!sidebarOpen) {
    return (
      <div className="w-12 bg-screen-alt border-r border-border flex flex-col items-center py-4 gap-4">
        <button
          onClick={toggleSidebar}
          className="text-foreground-secondary hover:text-foreground transition-colors"
        >
          <ChevronLeft size={20} className="rotate-180" />
        </button>
        <NavIcon active={activeView === "home"} onClick={() => setActiveView("home")}>
          <Home size={20} />
        </NavIcon>
        <NavIcon active={activeView === "chat"} onClick={() => setActiveView("chat")}>
          <MessageSquare size={20} />
        </NavIcon>
        <NavIcon active={activeView === "settings"} onClick={() => setActiveView("settings")}>
          <Settings size={20} />
        </NavIcon>
      </div>
    );
  }

  return (
    <div className="w-72 bg-screen-alt border-r border-border flex flex-col h-full">
      {/* Header */}
      <div className="px-4 py-4 border-b border-border flex items-center justify-between">
        <h1 className="text-base font-bold text-foreground tracking-tight">Console</h1>
        <button
          onClick={toggleSidebar}
          className="text-foreground-secondary hover:text-foreground transition-colors"
        >
          <ChevronLeft size={18} />
        </button>
      </div>

      {/* Nav items */}
      <div className="px-2 py-2 border-b border-border flex gap-1">
        <NavButton active={activeView === "home"} onClick={() => setActiveView("home")}>
          <Home size={16} /> Home
        </NavButton>
        <NavButton active={activeView === "chat"} onClick={() => setActiveView("chat")}>
          <MessageSquare size={16} /> Chat
        </NavButton>
        <NavButton active={activeView === "settings"} onClick={() => setActiveView("settings")}>
          <Settings size={16} />
        </NavButton>
      </div>

      {/* Projects + Sessions */}
      <div className="flex-1 overflow-y-auto px-2 py-3">
        <div className="flex items-center justify-between mb-2 px-2">
          <span className="text-xs font-semibold text-foreground-secondary uppercase tracking-wider">
            Projects
          </span>
          <span className="text-xs text-foreground-muted">{projects.length}</span>
        </div>

        {loading ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-sm text-foreground-muted">Loading...</span>
          </div>
        ) : projects.length === 0 ? (
          <div className="px-2 py-8 text-center">
            <Folder size={28} className="mx-auto text-foreground-muted mb-2" />
            <p className="text-xs text-foreground-muted">
              No projects yet. Add one from the Home tab.
            </p>
          </div>
        ) : (
          <div className="space-y-1.5">
            {projects.map((project) => {
              const isSelected = selectedProjectId === project.id;
              const sessions = sessionsByProject[project.id] ?? [];

              return (
                <div key={project.id}>
                  <button
                    onClick={() => handleProjectClick(project.id)}
                    className={`w-full flex items-center gap-2.5 px-3 py-2.5 rounded-lg transition-colors text-left ${
                      isSelected
                        ? "bg-white/10 border border-border"
                        : "hover:bg-white/5 border border-transparent"
                    }`}
                  >
                    <Folder size={16} className="shrink-0 text-foreground" />
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-foreground truncate">
                        {project.name}
                      </div>
                      <div className="text-xs text-foreground-muted font-mono truncate">
                        {project.path}
                      </div>
                    </div>
                  </button>

                  {isSelected && (
                    <div className="ml-4 mt-1 mb-2 space-y-1">
                      <div className="flex items-center justify-between px-2 py-1">
                        <span className="text-xs text-foreground-muted uppercase tracking-wider">
                          Sessions ({sessions.length})
                        </span>
                        <button
                          onClick={() => handleNewChat(project.id, project.path)}
                          className="text-xs font-bold text-foreground hover:text-white flex items-center gap-0.5 px-2 py-1 rounded-full border border-border hover:bg-white/10 transition-colors"
                        >
                          <Plus size={12} /> New
                        </button>
                      </div>

                      {sessions.length === 0 ? (
                        <p className="text-xs text-foreground-muted italic px-2 py-1">
                          No sessions yet.
                        </p>
                      ) : (
                        sessions.map((sess) => (
                          <SessionItem key={sess.id} session={sess} projectId={project.id} />
                        ))
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function SessionItem({
  session,
  projectId,
}: {
  session: SessionHeader;
  projectId: string;
}) {
  const { selectedSessionId, setSelectedSessionId, setActiveView } = useAppStore();
  const { deleteSession } = useProjectStore();
  const isActive = selectedSessionId === session.id;

  return (
    <div
      className={`group flex items-center justify-between px-3 py-2 rounded-lg cursor-pointer transition-colors ${
        isActive ? "bg-white/15 border border-border" : "hover:bg-white/5 border border-transparent"
      }`}
      onClick={() => {
        setSelectedSessionId(isActive ? null : session.id);
        if (!isActive) setActiveView("chat");
      }}
    >
      <div className="flex-1 min-w-0 pr-2">
        <div className={`text-xs font-medium truncate ${isActive ? "text-foreground" : "text-foreground-secondary"}`}>
          {session.title || "Untitled"}
        </div>
        <div className="text-xs text-foreground-muted font-mono truncate">
          {session.modelId}
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <span className="text-xs text-foreground-muted">
          {formatRelativeTime(session.createdAt)}
        </span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            deleteSession(session.id, projectId);
            if (isActive) setSelectedSessionId(null);
          }}
          className="text-foreground-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-opacity text-xs"
        >
          ✕
        </button>
      </div>
    </div>
  );
}

function NavButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`flex-1 flex items-center justify-center gap-1.5 px-2 py-2 rounded-lg text-xs font-medium transition-colors ${
        active ? "bg-white/10 text-foreground" : "text-foreground-secondary hover:text-foreground hover:bg-white/5"
      }`}
    >
      {children}
    </button>
  );
}

function NavIcon({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`p-2 rounded-lg transition-colors ${
        active ? "bg-white/10 text-foreground" : "text-foreground-secondary hover:text-foreground hover:bg-white/5"
      }`}
    >
      {children}
    </button>
  );
}
