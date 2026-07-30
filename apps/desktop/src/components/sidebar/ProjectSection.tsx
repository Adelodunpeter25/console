import React from "react";
import type { ProjectInfo, SessionHeader } from "@console/types";
import { Folder, FolderOpen, Plus } from "lucide-react";
import { useAppStore, useProjectStore } from "../../store";
import { SessionItem } from "./SessionItem";

interface ProjectSectionProps {
  project: ProjectInfo;
  sessions: SessionHeader[];
  isExpanded: boolean;
}

const INITIAL_VISIBLE_COUNT = 3;

/**
 * A single project group — folder header with collapsible session list.
 * Supports "Show N More" truncation as shown in the mockup screenshot.
 */
export function ProjectSection({ project, sessions, isExpanded }: ProjectSectionProps) {
  const { selectedSessionId, selectedProjectId, setSelectedProjectId, setSelectedSessionId, toggleProjectExpanded } =
    useAppStore();
  const { loadSessions, createSession, sessionsByProject } = useProjectStore();
  const [showAll, setShowAll] = React.useState(false);

  const isProjectActive = selectedProjectId === project.id;

  const handleToggle = () => {
    toggleProjectExpanded(project.id);
    setSelectedProjectId(isExpanded ? null : project.id);
    if (!isExpanded && !sessionsByProject[project.id]) {
      loadSessions(project.id);
    }
  };

  const handleNewChat = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!isExpanded) {
      toggleProjectExpanded(project.id);
    }
    const session = await createSession(project.path, project.id, "New Chat");
    setSelectedProjectId(project.id);
    setSelectedSessionId(session.id);
  };

  const visibleSessions = showAll ? sessions : sessions.slice(0, INITIAL_VISIBLE_COUNT);
  const hiddenCount = sessions.length - INITIAL_VISIBLE_COUNT;

  return (
    <div className="mb-2">
      {/* Project Header Row */}
      <div
        onClick={handleToggle}
        className={`group relative flex items-center justify-between px-3 py-1.5 rounded-md cursor-pointer select-none ${
          isProjectActive
            ? "text-foreground font-semibold"
            : "text-foreground-secondary hover:text-foreground"
        }`}
      >
        {/* Folder Icon + Name */}
        <div className="flex items-center gap-2.5 truncate flex-1 min-w-0 pr-2">
          {isExpanded ? (
            <FolderOpen size={16} className="text-foreground shrink-0" />
          ) : (
            <Folder size={16} className="text-foreground-muted shrink-0 group-hover:text-foreground transition-colors" />
          )}
          <span className="text-sm font-semibold truncate tracking-tight">
            {project.name}
          </span>
        </div>

        {/* Quick Add Chat Action */}
        <button
          onClick={handleNewChat}
          className="p-1 text-foreground-muted hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
          title="New chat in project"
        >
          <Plus size={14} />
        </button>
      </div>

      {/* Nested Sessions */}
      {isExpanded && (
        <div className="ml-4 pl-3.5 border-l border-white/10 mt-1 space-y-0.5">
          {sessions.length === 0 ? (
            <div
              onClick={handleNewChat}
              className="px-3 py-1.5 text-xs text-foreground-muted hover:text-foreground cursor-pointer transition-colors"
            >
              + Create new chat
            </div>
          ) : (
            <>
              {visibleSessions.map((sess) => (
                <SessionItem
                  key={sess.id}
                  session={sess}
                  projectId={project.id}
                  isActive={selectedSessionId === sess.id}
                />
              ))}

              {/* Show N More / Show Less */}
              {hiddenCount > 0 && !showAll && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setShowAll(true);
                  }}
                  className="px-3 py-1 text-xs font-medium text-foreground-muted hover:text-foreground transition-colors text-left"
                >
                  Show {hiddenCount} More
                </button>
              )}
              {showAll && hiddenCount > 0 && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setShowAll(false);
                  }}
                  className="px-3 py-1 text-xs font-medium text-foreground-muted hover:text-foreground transition-colors text-left"
                >
                  Show Less
                </button>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
