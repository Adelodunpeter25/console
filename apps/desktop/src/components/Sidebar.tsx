import React from "react";
import {
  Folder,
  ChevronRight,
  Plus,
  PanelLeftClose,
  PanelLeft,
  X,
  FolderOpen,
} from "lucide-react";
import type { SessionHeader, SessionStatus } from "@console/types";
import { useAppStore, useProjectStore } from "../store";
import { formatRelativeTime } from "../utils/time";

/**
 * Left sidebar — project and session navigator.
 *
 * Each project is a collapsible row with a folder icon. Expanding a project
 * loads and reveals its sessions with status dots (working/done/needs_attention).
 * A "+" button at the top adds new projects inline.
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
          onClick={() => setShowAddForm(true)}
          className="p-2 rounded-lg text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
          title="Add project"
        >
          <Plus size={18} />
        </button>
      </div>
    );
  }

  return (
    <div className="w-72 bg-sidebar border-r border-border flex flex-col h-full shrink-0">
      {/* Header */}
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

      {/* Project + session list */}
      <div className="flex-1 overflow-y-auto px-2 py-2">
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-sm text-foreground-muted">Loading...</span>
          </div>
        ) : projects.length === 0 ? (
          <div className="px-2 py-8 text-center">
            <Folder size={28} className="mx-auto text-foreground-muted mb-2" />
            <p className="text-xs text-foreground-muted">
              No projects yet. Click + to add one.
            </p>
          </div>
        ) : (
          <div className="space-y-0.5">
            {projects.map((project) => {
              const isExpanded = expandedProjects.has(project.id);
              const sessions = sessionsByProject[project.id] ?? [];

              return (
                <div key={project.id}>
                  {/* Project row */}
                  <button
                    onClick={() => handleProjectToggle(project.id)}
                    className={`w-full flex items-center gap-2 px-2.5 py-2 rounded-lg transition-colors text-left ${
                      isExpanded
                        ? "bg-white/8 border border-border"
                        : "hover:bg-white/5 border border-transparent"
                    }`}
                  >
                    <ChevronRight
                      size={14}
                      className={`text-foreground-muted shrink-0 transition-transform ${
                        isExpanded ? "rotate-90" : ""
                      }`}
                    />
                    <FolderOpen size={15} className="shrink-0 text-foreground" />
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-foreground truncate">
                        {project.name}
                      </div>
                    </div>
                  </button>

                  {/* Sessions (collapsible) */}
                  {isExpanded && (
                    <div className="relative ml-4 mt-0.5 mb-1.5 pl-3 space-y-0.5">
                      {/* Vertical tree line */}
                      <div className="absolute left-0 top-0 bottom-0 w-px bg-border" />

                      <div className="flex items-center justify-between px-2 py-1 relative">
                        <span className="text-xs text-foreground-muted uppercase tracking-wider">
                          {sessions.length} session{sessions.length !== 1 ? "s" : ""}
                        </span>
                        <button
                          onClick={() => handleNewChat(project.id, project.path)}
                          className="text-xs font-bold text-foreground-secondary hover:text-foreground flex items-center gap-0.5 px-2 py-0.5 rounded-full border border-border hover:bg-white/10 transition-colors"
                        >
                          <Plus size={11} /> New
                        </button>
                      </div>

                      {sessions.length === 0 ? (
                        <p className="text-xs text-foreground-muted italic px-2 py-1">
                          No sessions yet.
                        </p>
                      ) : (
                        sessions.map((sess) => (
                          <SessionItem
                            key={sess.id}
                            session={sess}
                            projectId={project.id}
                            isActive={selectedSessionId === sess.id}
                          />
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
      className={`group flex items-center gap-2 px-2.5 py-1.5 rounded-lg cursor-pointer transition-colors ${
        isActive ? "bg-white/12 border border-border" : "hover:bg-white/5 border border-transparent"
      }`}
      onClick={() => setSelectedSessionId(isActive ? null : session.id)}
    >
      {/* Status indicator */}
      {status === "working" ? (
        <div className="w-3.5 h-3.5 shrink-0 flex items-center justify-center">
          <div className="w-3 h-3 border-[1.5px] border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : (
        <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${STATUS_DOT[status]}`} />
      )}

      <div className="flex-1 min-w-0 flex items-baseline gap-2">
        <span
          className={`text-xs font-medium truncate ${
            isActive ? "text-foreground" : "text-foreground-secondary"
          }`}
        >
          {session.title || "Untitled"}
        </span>
        <span className="text-xs text-foreground-muted shrink-0">
          {formatRelativeTime(session.createdAt)}
        </span>
      </div>

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
  );
}
