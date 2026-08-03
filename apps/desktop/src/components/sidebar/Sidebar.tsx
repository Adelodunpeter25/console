import React from "react";
import { FolderOpen, Plus, SquarePen } from "lucide-react";
import { toast } from "sonner";
import { useAppStore, useProjectStore } from "../../store";
import { SessionItem } from "./SessionItem";

/**
 * Left sidebar — flat session list with New Chat action.
 */
export function Sidebar() {
  const { sidebarOpen, selectedSessionId, setSelectedProjectId, setSelectedSessionId } = useAppStore();
  const { projects, loading, loadProjects, sessions, sessionsLoading, loadSessions, createSession, addProject } =
    useProjectStore();

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  React.useEffect(() => {
    loadSessions();
  }, [loadSessions]);

  if (!sidebarOpen) return null;

  const handleGlobalNewChat = async () => {
    const targetProject = projects[0];
    if (!targetProject) {
      toast.error("Please add a project first.");
      return;
    }
    const session = await createSession(targetProject.path, targetProject.id, "New Chat");
    setSelectedProjectId(targetProject.id);
    setSelectedSessionId(session.id);
  };

  const handleNewProjectStub = async () => {
    const pathInput = window.prompt("Enter project directory path:", "~/Documents/projects/new-project");
    if (!pathInput) return;
    try {
      const newProj = await addProject(pathInput);
      toast.success(`Project '${newProj.name}' added!`);
    } catch {
      toast("New Project action triggered (stub).");
    }
  };

  return (
    <div className="w-72 bg-sidebar border-r border-border flex flex-col h-full shrink-0 select-none">
      {/* Top Actions Bar */}
      <div className="px-3 pt-3 pb-2 flex items-center justify-end shrink-0">
        <button
          onClick={handleGlobalNewChat}
          className="p-1.5 text-foreground-muted hover:text-foreground rounded-lg shrink-0 cursor-pointer"
          title="New Chat"
        >
          <SquarePen size={16} />
        </button>
      </div>

      {/* Category Header */}
      <div className="px-4 pt-2 pb-1">
        <span className="text-[11px] font-bold tracking-wider text-foreground-muted uppercase">
          CHATS
        </span>
      </div>

      {/* Session List */}
      <div className="flex-1 overflow-y-auto px-2 py-1 space-y-0.5">
        {loading || sessionsLoading ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-xs text-foreground-muted">Loading...</span>
          </div>
        ) : sessions.length === 0 ? (
          <div className="px-2 py-8 text-center">
            <FolderOpen size={24} className="mx-auto text-foreground-muted mb-2" />
            <p className="text-xs text-foreground-muted">No chats yet.</p>
          </div>
        ) : (
          sessions.map((session) => (
            <SessionItem key={session.id} session={session} isActive={session.id === selectedSessionId} />
          ))
        )}
      </div>

      {/* Bottom Footer Bar */}
      <div className="border-t border-border/80 p-3 flex items-center justify-between shrink-0">
        <button
          onClick={handleNewProjectStub}
          className="flex items-center gap-2 text-xs font-medium text-foreground-secondary hover:text-foreground transition-colors"
        >
          <Plus size={14} />
          <span>New Project</span>
        </button>
      </div>
    </div>
  );
}
