import type { ProjectInfo, SessionHeader } from "@console/types";
import { ChevronDown, ChevronRight, Plus } from "lucide-react";
import { useAppStore, useProjectStore } from "../../store";
import { SessionItem } from "./SessionItem";

interface ProjectSectionProps {
  project: ProjectInfo;
  sessions: SessionHeader[];
  isExpanded: boolean;
}

/**
 * A single project group — collapsible header with session list inside.
 */
export function ProjectSection({ project, sessions, isExpanded }: ProjectSectionProps) {
  const { selectedSessionId, setSelectedProjectId, setSelectedSessionId, toggleProjectExpanded } =
    useAppStore();
  const { loadSessions, createSession, sessionsByProject } = useProjectStore();

  const handleToggle = () => {
    toggleProjectExpanded(project.id);
    setSelectedProjectId(isExpanded ? null : project.id);
    if (!isExpanded && !sessionsByProject[project.id]) {
      loadSessions(project.id);
    }
  };

  const handleNewChat = async () => {
    const session = await createSession(project.path, project.id, "New Chat");
    setSelectedSessionId(session.id);
  };

  return (
    <div className="border-b border-border/50">
      {/* Project header */}
      <button
        onClick={handleToggle}
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
            onClick={handleNewChat}
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
}
