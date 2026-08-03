import React from "react";
import type { ProjectInfo } from "@console/types";
import { Dropdown, DropdownItem } from "./Dropdown";

interface ProjectSelectorProps {
  /** Backend projects to choose from. */
  projects: ProjectInfo[];
  /** Id of the currently selected project, if known. */
  selectedId: string | null;
  /** Label shown when no project is selected. */
  fallbackLabel: string;
  onSelect: (project: ProjectInfo) => void;
}

/**
 * Dropdown for selecting the working folder of a session. The list comes
 * from the backend projects endpoint via the project store.
 */
export function ProjectSelector({ projects, selectedId, fallbackLabel, onSelect }: ProjectSelectorProps) {
  const selected = projects.find((p) => p.id === selectedId) ?? null;

  return (
    <Dropdown label={selected ? selected.name : fallbackLabel} heading="Project" width={280}>
      {projects.length === 0 ? (
        <div className="px-3 py-4 text-center text-xs text-foreground-muted">
          No projects yet — add one from the sidebar.
        </div>
      ) : (
        projects.map((project) => (
          <DropdownItem
            key={project.id}
            selected={selectedId === project.id}
            onClick={() => onSelect(project)}
          >
            {project.name}
          </DropdownItem>
        ))
      )}
    </Dropdown>
  );
}
