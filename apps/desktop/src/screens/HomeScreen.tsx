import React from "react";
import { Folder, Plus, FolderOpen } from "lucide-react";
import { useAppStore, useProjectStore } from "../store";
import { GlassSurface } from "../components/GlassSurface";

export function HomeScreen() {
  const { setSelectedProjectId, setSelectedSessionId, setActiveView } = useAppStore();
  const { projects, loading, loadProjects, addProject } = useProjectStore();
  const [addingPath, setAddingPath] = React.useState("");
  const [showAddForm, setShowAddForm] = React.useState(false);
  const [adding, setAdding] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  const handleAdd = async () => {
    if (!addingPath.trim()) return;
    setAdding(true);
    setError(null);
    try {
      const project = await addProject(addingPath.trim());
      setSelectedProjectId(project.id);
      setShowAddForm(false);
      setAddingPath("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to add project");
    } finally {
      setAdding(false);
    }
  };

  const handleSelectProject = (projectId: string) => {
    setSelectedProjectId(projectId);
    setSelectedSessionId(null);
    setActiveView("chat");
  };

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6 bg-screen">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold text-foreground tracking-tight">Console Workspace</h2>
          <p className="text-sm text-foreground-secondary mt-1">
            Manage your repositories & AI agent sessions
          </p>
        </div>
        <button
          onClick={() => setShowAddForm(!showAddForm)}
          className="bg-transparent border border-border px-4 py-2 rounded-full text-sm font-semibold text-foreground hover:bg-white/10 transition-colors flex items-center gap-1.5"
        >
          <Plus size={16} /> Add Project
        </button>
      </div>

      {showAddForm && (
        <GlassSurface className="mb-5">
          <div className="flex items-center gap-3">
            <FolderOpen size={20} className="text-foreground shrink-0" />
            <input
              type="text"
              value={addingPath}
              onChange={(e) => setAddingPath(e.target.value)}
              placeholder="/path/to/your/project"
              className="flex-1 bg-card-alt border border-border rounded-xl px-4 py-2.5 text-sm text-foreground font-mono outline-none focus:border-white/30 transition-colors"
              onKeyDown={(e) => e.key === "Enter" && handleAdd()}
              autoFocus
            />
            <button
              onClick={handleAdd}
              disabled={adding || !addingPath.trim()}
              className="bg-white text-black px-5 py-2.5 rounded-xl text-sm font-bold disabled:opacity-30 hover:bg-white/90 transition-colors"
            >
              {adding ? "Adding..." : "Add"}
            </button>
            <button
              onClick={() => {
                setShowAddForm(false);
                setError(null);
              }}
              className="text-foreground-secondary hover:text-foreground text-sm px-3"
            >
              Cancel
            </button>
          </div>
          {error && <p className="text-sm text-danger mt-2">{error}</p>}
        </GlassSurface>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <span className="text-sm text-foreground-muted">Loading projects...</span>
        </div>
      ) : projects.length === 0 ? (
        <GlassSurface className="flex flex-col items-center justify-center py-14">
          <Folder size={40} className="text-foreground mb-4" />
          <p className="text-foreground text-base font-semibold">No Projects Added</p>
          <p className="text-foreground-secondary text-sm text-center mt-2 max-w-xs">
            Enter a path to your project directory above to get started.
          </p>
          <button
            onClick={() => setShowAddForm(true)}
            className="mt-6 bg-white text-black py-3 px-6 rounded-full text-sm font-bold hover:bg-white/90 transition-colors"
          >
            Add Project Path
          </button>
        </GlassSurface>
      ) : (
        <div className="space-y-3.5">
          {projects.map((project) => (
            <GlassSurface key={project.id} className="p-0 overflow-hidden">
              <button
                onClick={() => handleSelectProject(project.id)}
                className="w-full flex items-center justify-between p-4 hover:bg-white/5 transition-colors text-left"
              >
                <div className="flex items-center gap-3.5 flex-1 pr-2">
                  <Folder size={22} className="text-foreground shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="text-base font-semibold text-foreground">
                      {project.name}
                    </div>
                    <div className="text-xs text-foreground-muted font-mono mt-0.5 truncate">
                      {project.path}
                    </div>
                  </div>
                </div>
                <span className="text-foreground-secondary text-sm">›</span>
              </button>
            </GlassSurface>
          ))}
        </div>
      )}
    </div>
  );
}
