import React, { useState } from "react";
import {
  useProjects,
  useAddProject,
  useSessions,
  useCreateSession,
  useDeleteSession,
} from "@console/api";
import { globalState$ } from "../state/global-state.js";
import { observer } from "@legendapp/state/react";
import {
  FolderPlus,
  MessageSquarePlus,
  Trash2,
  ChevronDown,
  FolderOpen,
  Terminal,
  X,
  Plus,
  Sparkles,
  MessageSquare,
} from "lucide-react";

export const ProjectSidebar = observer(() => {
  const { data: projects = [] } = useProjects();
  const addProjectMutation = useAddProject();
  const createSessionMutation = useCreateSession();
  const deleteSessionMutation = useDeleteSession();

  const activeProjectId = globalState$.activeProjectId.get();
  const activeSessionId = globalState$.activeSessionId.get();
  const isOpen = globalState$.isSidebarOpen.get();

  const [newProjectPath, setNewProjectPath] = useState("");
  const [showAddProjectModal, setShowAddProjectModal] = useState(false);
  const [newSessionTitle, setNewSessionTitle] = useState("");
  const [showAddSession, setShowAddSession] = useState(false);

  const { data: sessions = [] } = useSessions(
    activeProjectId ? { projectId: activeProjectId } : undefined,
  );

  const handleAddProject = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newProjectPath.trim()) return;
    try {
      const added = await addProjectMutation.mutateAsync(newProjectPath);
      globalState$.activeProjectId.set(added.id);
      setNewProjectPath("");
      setShowAddProjectModal(false);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to add project");
    }
  };

  const handleCreateSession = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!activeProjectId) return;
    try {
      const activeProject = projects.find((p) => p.id === activeProjectId);
      const created = await createSessionMutation.mutateAsync({
        cwd: activeProject?.path || "",
        projectId: activeProjectId,
        title: newSessionTitle.trim() || "New Session",
      });
      globalState$.activeSessionId.set(created.id);
      setNewSessionTitle("");
      setShowAddSession(false);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to create session");
    }
  };

  const handleDeleteSession = async (sessionId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (!confirm("Delete session?")) return;
    try {
      await deleteSessionMutation.mutateAsync(sessionId);
      if (activeSessionId === sessionId) {
        globalState$.activeSessionId.set(null);
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to delete session");
    }
  };

  if (!isOpen) return null;

  const activeProject = projects.find((p) => p.id === activeProjectId);

  return (
    <aside className="w-72 h-screen sidebar-surface flex flex-col z-30 shrink-0 select-none">
      {/* Header Titlebar */}
      <div className="h-12 px-3 border-b border-border/40 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="w-6 h-6 rounded-md bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
            <Terminal size={14} />
          </div>
          <span className="font-semibold text-sm tracking-tight text-foreground">Console</span>
        </div>
        <button
          onClick={() => globalState$.isSidebarOpen.set(false)}
          className="p-1 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground cursor-pointer transition-colors"
        >
          <X size={15} />
        </button>
      </div>

      {/* Active Project Dropdown */}
      <div className="p-3 border-b border-border/30 bg-card/20">
        <div className="flex items-center justify-between mb-1.5 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
          <span>Project</span>
          <button
            onClick={() => setShowAddProjectModal(true)}
            className="hover:text-foreground cursor-pointer flex items-center gap-1 text-[11px] capitalize text-primary font-normal"
          >
            <Plus size={12} /> Add
          </button>
        </div>

        <div className="relative">
          <select
            value={activeProjectId || ""}
            onChange={(e) => globalState$.activeProjectId.set(e.target.value || null)}
            className="w-full bg-card/60 border border-border/60 rounded-md px-2.5 py-1.5 text-xs text-foreground appearance-none cursor-pointer focus:outline-none focus:border-primary/50"
          >
            <option value="" disabled>Select Project...</option>
            {projects.map((proj) => (
              <option key={proj.id} value={proj.id}>
                {proj.name}
              </option>
            ))}
          </select>
          <ChevronDown size={14} className="absolute right-2.5 top-2.5 text-muted-foreground pointer-events-none" />
        </div>
      </div>

      {/* Sessions List */}
      <div className="flex-1 overflow-y-auto p-2">
        <div className="px-2 py-1 flex items-center justify-between mb-1">
          <span className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
            Sessions
          </span>
          {activeProjectId && (
            <button
              onClick={() => setShowAddSession(true)}
              className="p-1 rounded hover:bg-accent text-muted-foreground hover:text-foreground cursor-pointer"
              title="New Chat Session"
            >
              <MessageSquarePlus size={14} />
            </button>
          )}
        </div>

        {/* Modal/Input for New Session */}
        {showAddSession && (
          <form onSubmit={handleCreateSession} className="mb-2 p-2 rounded-md bg-card/80 border border-border/60">
            <input
              type="text"
              placeholder="Session Title..."
              value={newSessionTitle}
              onChange={(e) => setNewSessionTitle(e.target.value)}
              className="w-full bg-background/50 border border-border/40 rounded px-2 py-1 text-xs text-foreground focus:outline-none mb-2"
              autoFocus
            />
            <div className="flex justify-end gap-1.5">
              <button
                type="button"
                onClick={() => setShowAddSession(false)}
                className="px-2 py-1 text-[11px] rounded hover:bg-accent text-muted-foreground"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-2 py-1 text-[11px] rounded bg-primary text-primary-foreground font-medium"
              >
                Create
              </button>
            </div>
          </form>
        )}

        {!activeProjectId ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            Select a project above to view sessions
          </div>
        ) : sessions.length === 0 ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            No sessions yet. Click + to create one.
          </div>
        ) : (
          <div className="flex flex-col gap-0.5">
            {sessions.map((sess) => {
              const isActive = sess.id === activeSessionId;
              return (
                <div
                  key={sess.id}
                  onClick={() => globalState$.activeSessionId.set(sess.id)}
                  className={`group px-2.5 py-1.5 rounded-md text-xs flex items-center justify-between cursor-pointer transition-colors ${
                    isActive
                      ? "bg-accent/80 text-foreground font-medium border border-border/50"
                      : "text-muted-foreground hover:bg-accent/40 hover:text-foreground"
                  }`}
                >
                  <div className="flex items-center gap-2 overflow-hidden">
                    <MessageSquare size={13} className={isActive ? "text-primary" : "text-muted-foreground"} />
                    <span className="truncate">{sess.title || "Untitled Session"}</span>
                  </div>
                  <button
                    onClick={(e) => handleDeleteSession(sess.id, e)}
                    className="opacity-0 group-hover:opacity-100 p-1 hover:text-destructive transition-opacity"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Project Add Modal */}
      {showAddProjectModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-xs flex items-center justify-center p-4">
          <div className="bg-card border border-border/80 rounded-lg w-full max-w-sm p-4 shadow-xl">
            <h3 className="font-medium text-sm text-foreground mb-3 flex items-center gap-2">
              <FolderPlus size={16} className="text-primary" /> Add Local Workspace Project
            </h3>
            <form onSubmit={handleAddProject}>
              <input
                type="text"
                placeholder="/absolute/path/to/project"
                value={newProjectPath}
                onChange={(e) => setNewProjectPath(e.target.value)}
                className="w-full bg-background border border-border rounded-md px-3 py-1.5 text-xs text-foreground focus:outline-none focus:border-primary/60 mb-3"
                autoFocus
              />
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setShowAddProjectModal(false)}
                  className="px-3 py-1.5 text-xs rounded-md border border-border text-muted-foreground hover:text-foreground hover:bg-accent"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="px-3 py-1.5 text-xs rounded-md bg-primary text-primary-foreground font-medium"
                >
                  Add Project
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </aside>
  );
});
