import React from "react";
import {
  FolderOpen,
  ChevronDown,
  ChevronRight,
  Plus,
  X,
  PanelLeftClose,
  PanelLeft,
} from "lucide-react";
import type { SessionHeader, SessionStatus } from "@console/types";
import { useAppStore, useProjectStore } from "../store";
import { formatRelativeTime } from "../utils/time";

/**
 * Left sidebar — Conductor-style project navigator.
 *
 * Each project is a section with a bold header and a chevron to
 * collapse/expand. Sessions are listed below with a status indicator,
 * truncating title, and right-aligned timestamp.
 */
export function Sidebar() {
  const {
    selectedProjectId,
    setSelectedProjectId,
    selectedSessionId,
    setSelectedSessionId,
    sidebarOpen,
    toggleSidebar,
    expandedProjects,
    toggleProjectExpanded,
  } = useAppStore();

  const {
    projects,
    loading,
    loadProjects,
    sessionsByProject,
    loadSessions,
    createSession,
    addProject,
  } = useProjectStore();

  const [showAddForm, setShowAddForm] = React.useState(false);
  const [addingPath, setAddingPath] = React.useState("");
  const [adding, setAdding] = React.useState(false);
  const [addError, setAddError] = React.useState<string | null>(null);

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  const handleProjectToggle = (projectId: string) => {
    const isExpanded = expandedProjects.has(projectId);
    toggleProjectExpanded(projectId);
    setSelectedProjectId(isExpanded ? null : projectId);
    if (!isExpanded && !sessionsByProject[projectId]) {
      loadSessions(projectId);
    }
  };

  const handleNewChat = async (projectId: string, cwd: string) => {
    const session = await createSession(cwd, projectId, "New Chat");
    setSelectedSessionId(session.id);
  };

  const handleAddProject = async () => {
    if (!addingPath.trim()) return;
    setAdding(true);
    setAddError(null);
    try {
      const project = await addProject(addingPath.trim());
      setSelectedProjectId(project.id);
      toggleProjectExpanded(project.id);
      setShowAddForm(false);
      setAddingPath("");
    } catch (e) {
      setAddError(e instanceof Error ? e.message : "Failed to add project");
    } finally {
      setAdding(false);
    }
  };

  // Collapsed rail
  if (!sidebarOpen) {
    return (
      <div className="w-12 bg-sidebar border-r border-border flex flex-col items-center py-3 gap-3 shrink-0">
        <button
          onClick={toggleSidebar}
          className="p-2 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
          title="Expand sidebar"
        >
          <PanelLeft size={18} />
        </button>
        <button
          onClick={() => {
            setShowAddForm(true);
            setSidebarOpenViaToggle();
          }}
          className="p-2 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
          title="Add project"
        >
          <Plus size={18} />
        </button>
      </div>
    );
  }

  function setSidebarOpenViaToggle() {
    toggleSidebar();
  }

  return (
    <div className="w-72 bg-sidebar border-r border-border flex flex-col h-full shrink-0">
      {/* Top bar */}
      <div className="px-4 h-12 flex items-center justify-between border-b border-border shrink-0">
        <span className="text-sm font-bold text-foreground tracking-tight">Projects</span>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setShowAddForm(true)}
            className="p-1.5 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
            title="Add project"
          >
            <Plus size={16} />
          </button>
          <button
            onClick={toggleSidebar}
            className="p-1.5 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
            title="Collapse sidebar"
          >
            <PanelLeftClose size={16} />
          </button>
        </div>
      </div>

      {/* Add project form */}
      {showAddForm && (
        <div className="px-3 py-3 border-b border-border shrink-0">
          <div className="flex items-center gap-2 mb-2">
            <FolderOpen size={16} className="text-foreground shrink-0" />
            <input
              type="text"
              value={addingPath}
              onChange={(e) => setAddingPath(e.target.value)}
              placeholder="/path/to/project"
              className="flex-1 bg-card-alt border border-border rounded-lg px-3 py-2 text-xs text-foreground font-mono outline-none focus:border-border-strong transition-colors"
              onKeyDown={(e) => {
                if (e.key === "Enter") handleAddProject();
                if (e.key === "Escape") {
                  setShowAddForm(false);
                  setAddingPath("");
                  setAddError(null);
                }
              }}
              autoFocus
            />
            <button
              onClick={() => {
                setShowAddForm(false);
                setAddingPath("");
                setAddError(null);
              }}
              className="p-1.5 text-foreground-muted hover:text-foreground transition-colors"
            >
              <X size={14} />
            </button>
          </div>
          <button
            onClick={handleAddProject}
            disabled={adding || !addingPath.trim()}
            className="w-full bg-white text-black rounded-lg py-2 text-xs font-bold disabled:opacity-30 hover:bg-white/90 transition-colors"
          >
            {adding ? "Adding..." : "Add Project"}
          </button>
          {addError && <p className="text-xs text-danger mt-2">{addError}</p>}
        </div>
      )}

      {/* Project sections */}
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-sm text-foreground-muted">Loading...</span>
          </div>
        ) : projects.length === 0 ? (
          <div className="px-2 py-8 text-center">
            <FolderOpen size={28} className="mx-auto text-foreground-muted mb-2" />
            <p className="text-xs text-foreground-muted">
              No projects yet. Click + to add one.
            </p>
          </div>
        ) : (
          projects.map((project) => {
            const isExpanded = expandedProjects.has(project.id);
            const sessions = sessionsByProject[project.id] ?? [];

            return (
              <div key={project.id} className="border-b border-border/50">
                {/* Project header */}
                <button
                  onClick={() => handleProjectToggle(project.id)}
                  className="w-full flex items-center justify-between px-4 py-2.5 hover:bg-white/[0.03] transition-colors"
                >
                  <span className="text-sm font-semibold text-foreground truncate">
                    {project.name}
                  </span>
                  {isExpanded ? (
                    <ChevronDown size={15} className="text-foreground-muted shrink-0 ml-2" />
                  ) : (
                    <ChevronRight size={15} className="text-foreground-muted shrink-0 ml-2" />
                  )}
                </button>

                {/* Sessions */}
                {isExpanded && (
                  <div className="pb-2">
                    {/* New chat button */}
                    <button
                      onClick={() => handleNewChat(project.id, project.path)}
                      className="w-full flex items-center gap-2 px-6 py-1.5 text-xs font-medium text-foreground-secondary hover:text-foreground transition-colors"
                    >
                      <Plus size={13} />
                      New Chat
                    </button>

                    {/* Session list */}
                    {sessions.length > 0 && (
                      <div className="mt-0.5">
                        {sessions.map((sess) => (
                          <SessionItem
                            key={sess.id}
                            session={sess}
                            projectId={project.id}
                            isActive={selectedSessionId === sess.id}
                          />
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>

    </div>
  );
}

const STATUS_DOT: Record<SessionStatus, string> = {
  idle: "bg-foreground-muted",
  working: "bg-blue-500",
  done: "bg-green-500",
  needs_attention: "bg-amber-500",
};

function SessionItem({
  session,
  projectId,
  isActive,
}: {
  session: SessionHeader;
  projectId: string;
  isActive: boolean;
}) {
  const { setSelectedSessionId } = useAppStore();
  const { deleteSession } = useProjectStore();
  const status = session.status ?? "idle";

  return (
    <div
      className={`group mx-2 flex flex-col gap-0.5 px-3 py-1.5 rounded-lg cursor-pointer transition-colors ${
        isActive ? "bg-white/10" : "hover:bg-white/[0.04]"
      }`}
      onClick={() => setSelectedSessionId(isActive ? null : session.id)}
    >
      {/* Row 1: status indicator + title + timestamp */}
      <div className="flex items-center gap-2">
        {status === "working" ? (
          <div className="w-3.5 h-3.5 shrink-0 flex items-center justify-center">
            <div className="w-3 h-3 border-[1.5px] border-blue-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${STATUS_DOT[status]}`} />
        )}

        <span
          className={`text-xs font-medium truncate flex-1 ${
            isActive ? "text-foreground" : "text-foreground-secondary"
          }`}
        >
          {session.title || "Untitled"}
        </span>

        <span className="text-xs text-foreground-muted shrink-0">
          {formatRelativeTime(session.createdAt)}
        </span>

        <button
          onClick={(e) => {
            e.stopPropagation();
            deleteSession(session.id, projectId);
            if (isActive) setSelectedSessionId(null);
          }}
          className="text-foreground-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
        >
          <X size={12} />
        </button>
      </div>

      {/* Row 2: branch name */}
      <span className="text-xs text-foreground-muted font-mono pl-6">
        main
      </span>
    </div>
  );
}
