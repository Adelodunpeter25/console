import React, { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useProjects, useAddProject, useSessions, useCreateSession, useDeleteSession } from "@console/api";
import { globalState$ } from "../state/global-state.js";
import { observer } from "@legendapp/state/react";
import { 
  FolderPlus, 
  MessageSquarePlus, 
  Trash2, 
  ChevronRight, 
  Folder, 
  Terminal,
  Layers,
  Menu,
  X
} from "lucide-react";

export const ProjectSidebar = observer(() => {
  const { data: projects = [], isLoading: isLoadingProjects } = useProjects();
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

  // Fetch sessions for active project
  const { data: sessions = [] } = useSessions(
    activeProjectId ? { projectId: activeProjectId } : undefined
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
      const activeProject = projects.find(p => p.id === activeProjectId);
      const created = await createSessionMutation.mutateAsync({
        cwd: activeProject?.path || "",
        projectId: activeProjectId,
        title: newSessionTitle.trim() || "New Chat Session"
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
    if (!confirm("Are you sure you want to delete this session?")) return;
    try {
      await deleteSessionMutation.mutateAsync(sessionId);
      if (activeSessionId === sessionId) {
        globalState$.activeSessionId.set(null);
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to delete session");
    }
  };

  if (!isOpen) {
    return (
      <button 
        onClick={() => globalState$.isSidebarOpen.set(true)}
        className="fixed top-4 left-4 z-40 p-2 rounded-lg bg-card border border-border text-foreground hover:bg-accent cursor-pointer transition-colors"
      >
        <Menu size={20} />
      </button>
    );
  }

  const activeProject = projects.find(p => p.id === activeProjectId);

  return (
    <aside className="w-80 h-screen border-r border-border bg-card flex flex-col z-30 shrink-0 text-foreground transition-all duration-300">
      {/* Sidebar Header */}
      <div className="p-4 border-b border-border flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Terminal className="text-primary w-5 h-5" />
          <span className="font-semibold text-lg tracking-wide text-primary">Console Agent</span>
        </div>
        <button 
          onClick={() => globalState$.isSidebarOpen.set(false)}
          className="p-1.5 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground cursor-pointer"
        >
          <X size={18} />
        </button>
      </div>

      {/* Projects Dropdown / Selector */}
      <div className="p-4 border-b border-border flex flex-col gap-2">
        <div className="flex items-center justify-between text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          <span>Active Project</span>
          <button 
            onClick={() => setShowAddProjectModal(true)}
            className="p-1 rounded hover:bg-accent hover:text-foreground cursor-pointer transition-colors"
            title="Register Project Directory"
          >
            <FolderPlus size={16} />
          </button>
        </div>

        {showAddProjectModal ? (
          <form onSubmit={handleAddProject} className="flex flex-col gap-2 mt-2">
            <input 
              type="text"
              placeholder="Absolute folder path..."
              value={newProjectPath}
              onChange={(e) => setNewProjectPath(e.target.value)}
              className="px-3 py-1.5 text-sm rounded bg-background border border-border text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
              required
            />
            <div className="flex gap-2 justify-end">
              <button 
                type="button" 
                onClick={() => setShowAddProjectModal(false)}
                className="px-2.5 py-1 text-xs rounded hover:bg-accent cursor-pointer"
              >
                Cancel
              </button>
              <button 
                type="submit"
                className="px-2.5 py-1 text-xs rounded bg-primary text-primary-foreground font-semibold hover:opacity-90 cursor-pointer"
              >
                Add
              </button>
            </div>
          </form>
        ) : (
          <select 
            value={activeProjectId || ""}
            onChange={(e) => {
              const val = e.target.value;
              globalState$.activeProjectId.set(val || null);
              globalState$.activeSessionId.set(null);
            }}
            className="w-full px-3 py-2 rounded bg-background border border-border text-foreground text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="">Select a workspace...</option>
            {projects.map((proj) => (
              <option key={proj.id} value={proj.id}>
                {proj.name} ({proj.path})
              </option>
            ))}
          </select>
        )}
      </div>

      {/* Sessions List */}
      <div className="flex-1 overflow-y-auto p-4 flex flex-col gap-4">
        {activeProjectId ? (
          <>
            <div className="flex items-center justify-between text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              <span>Chat Sessions</span>
              <button 
                onClick={() => setShowAddSession(true)}
                className="p-1 rounded hover:bg-accent hover:text-foreground cursor-pointer transition-colors"
                title="New Chat Session"
              >
                <MessageSquarePlus size={16} />
              </button>
            </div>

            {showAddSession && (
              <form onSubmit={handleCreateSession} className="flex flex-col gap-2 p-2 rounded bg-background border border-border">
                <input 
                  type="text"
                  placeholder="Session title..."
                  value={newSessionTitle}
                  onChange={(e) => setNewSessionTitle(e.target.value)}
                  className="px-2.5 py-1 text-xs rounded bg-card border border-border text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
                />
                <div className="flex gap-2 justify-end">
                  <button 
                    type="button" 
                    onClick={() => setShowAddSession(false)}
                    className="px-2 py-0.5 text-[10px] rounded hover:bg-accent cursor-pointer"
                  >
                    Cancel
                  </button>
                  <button 
                    type="submit"
                    className="px-2 py-0.5 text-[10px] rounded bg-primary text-primary-foreground font-semibold hover:opacity-90 cursor-pointer"
                  >
                    Create
                  </button>
                </div>
              </form>
            )}

            <div className="flex flex-col gap-1">
              {sessions.length === 0 ? (
                <div className="text-xs text-muted-foreground text-center py-6">No sessions yet. Start a new one!</div>
              ) : (
                sessions.map((sess) => (
                  <div 
                    key={sess.id}
                    onClick={() => globalState$.activeSessionId.set(sess.id)}
                    className={`group flex items-center justify-between px-3 py-2.5 rounded-md cursor-pointer transition-all text-sm ${
                      activeSessionId === sess.id 
                        ? "bg-accent text-accent-foreground font-medium" 
                        : "hover:bg-accent/50 text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    <div className="flex items-center gap-2 overflow-hidden">
                      <ChevronRight size={14} className="shrink-0" />
                      <span className="truncate">{sess.title}</span>
                    </div>
                    <button 
                      onClick={(e) => handleDeleteSession(sess.id, e)}
                      className="opacity-0 group-hover:opacity-100 p-1 rounded hover:bg-accent text-muted-foreground hover:text-destructive cursor-pointer transition-all"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))
              )}
            </div>
          </>
        ) : (
          <div className="text-sm text-muted-foreground text-center py-10 flex flex-col items-center gap-2">
            <Layers className="w-8 h-8 opacity-40" />
            <span>Select a project workspace above to view its chat logs and options.</span>
          </div>
        )}
      </div>

      {/* Sidebar Footer */}
      {activeProject && (
        <div className="p-4 border-t border-border bg-background/50 flex flex-col gap-1.5 text-xs text-muted-foreground">
          <div className="font-semibold text-foreground truncate">{activeProject.name}</div>
          <div className="truncate font-mono text-[10px]">{activeProject.path}</div>
        </div>
      )}
    </aside>
  );
});
