import React from "react";
import { FolderOpen } from "lucide-react";
import type { ProjectInfo } from "@console/types";
import { toast } from "sonner";
import { useProjectStore } from "../../store/useProjectStore";
import { api } from "../../lib/api";
import {
  Dropdown,
  DropdownAction,
  DropdownItem,
  DropdownSearch,
  DropdownSeparator,
} from "./Dropdown";

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
export function ProjectSelector({
  projects,
  selectedId,
  fallbackLabel,
  onSelect,
}: ProjectSelectorProps) {
  const selected = projects.find((p) => p.id === selectedId) ?? null;
  const addProject = useProjectStore((state) => state.addProject);
  const [search, setSearch] = React.useState("");
  const query = search.trim().toLowerCase();
  const filteredProjects = React.useMemo(
    () =>
      projects.filter(
        (project) =>
          !query ||
          project.name.toLowerCase().includes(query) ||
          project.path.toLowerCase().includes(query),
      ),
    [projects, query],
  );

  const handleOpenFolder = async () => {
    try {
      const picked = await api.pickFolder();
      if (!picked.path) return;
      const project = await addProject(picked.path);
      onSelect(project);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Unable to open folder");
    }
  };

  return (
    <Dropdown label={selected ? selected.name : fallbackLabel} heading="Project" width={280}>
      <DropdownSearch value={search} onChange={setSearch} placeholder="Search folders..." />
      {filteredProjects.length === 0 ? (
        <div className="px-3 py-4 text-center text-xs text-foreground-muted">
          {projects.length === 0 ? "No projects yet" : "No matching folders"}
        </div>
      ) : (
        filteredProjects.map((project) => (
          <DropdownItem
            key={project.id}
            selected={selectedId === project.id}
            onClick={() => onSelect(project)}
          >
            {project.name}
          </DropdownItem>
        ))
      )}
      <DropdownSeparator />
      <DropdownAction onClick={handleOpenFolder}>
        <FolderOpen size={14} />
        <span>Open folder...</span>
      </DropdownAction>
    </Dropdown>
  );
}
