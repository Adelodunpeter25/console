import React from "react";
import { Command } from "cmdk";
import {
  FolderPlus,
  MessageSquarePlus,
  TerminalSquare,
  ArrowLeft,
  Folder,
  File,
  ChevronRight,
  CornerDownLeft,
  Search,
} from "lucide-react";
import { toast } from "sonner";
import { useAppStore } from "../../store/useAppStore";
import { useProjectStore } from "../../store/useProjectStore";
import { useFsStore } from "../../store/useFsStore";
import { useTerminalStore } from "../../store/useTerminalStore";
import { useWorkspaceStore } from "../../layout/useWorkspaceStore";

interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type View = "root" | "browse";

/**
 * Raycast-style command palette built on cmdk.
 *
 * Root view lists commands (New Project, New Chat, New Terminal).
 * Selecting "New Project" pushes to a browse view that navigates the
 * filesystem via the backend browse API — arrow keys to move, Enter to
 * descend into a directory or select it, Back to return to the root.
 */
export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const [view, setView] = React.useState<View>("root");
  const [search, setSearch] = React.useState("");
  const [browsePath, setBrowsePath] = React.useState<string | undefined>(undefined);

  const browse = useFsStore((state) => state.browse);
  const browsing = useFsStore((state) => state.browsing);
  const browseDirectory = useFsStore((state) => state.browseDirectory);
  const addProject = useProjectStore((state) => state.addProject);
  const createSession = useProjectStore((state) => state.createSession);
  const projects = useProjectStore((state) => state.projects);
  const setSelectedProjectId = useAppStore((state) => state.setSelectedProjectId);
  const activeProjectId = useAppStore((state) => state.selectedProjectId);
  const openChatTab = useWorkspaceStore((state) => state.openChatTab);
  const openTerminalTab = useTerminalStore((state) => state.openTerminalTab);

  // Load root directory listing when entering browse view
  React.useEffect(() => {
    if (open && view === "browse") {
      browseDirectory(browsePath).catch(() => {});
    }
  }, [open, view, browsePath, browseDirectory]);

  // Reset to root when palette closes
  React.useEffect(() => {
    if (!open) {
      const timer = setTimeout(() => {
        setView("root");
        setSearch("");
        setBrowsePath(undefined);
      }, 200);
      return () => clearTimeout(timer);
    }
  }, [open]);

  const handleSelectProject = async (path: string) => {
    try {
      const project = await addProject(path);
      setSelectedProjectId(project.id);
      onOpenChange(false);
    } catch {
      // error is surfaced via store
    }
  };

  const handleNewChat = async () => {
    try {
      const session = await createSession("", "", "New Chat");
      setSelectedProjectId(null);
      openChatTab({
        type: "chat",
        projectId: "",
        sessionId: session.id,
        title: session.title,
      });
      onOpenChange(false);
    } catch {
      // error is surfaced via store
    }
  };

  const handleNewTerminal = async () => {
    try {
      const cwd = activeProjectId
        ? projects.find((p) => p.id === activeProjectId)?.path ?? ""
        : "";

      if (!activeProjectId || !cwd) {
        toast.error("Select an active project first to open a terminal.");
        return;
      }

      await openTerminalTab({ projectId: activeProjectId, cwd });
      onOpenChange(false);
    } catch (err) {
      toast.error(
        err instanceof Error ? err.message : "Failed to open terminal. Is the server running?",
      );
    }
  };

  const directories = React.useMemo(
    () =>
      (browse?.entries ?? []).filter((e) => e.isDir).sort((a, b) => a.name.localeCompare(b.name)),
    [browse],
  );

  return (
    <Command.Dialog
      open={open}
      onOpenChange={onOpenChange}
      label="Command Palette"
      className="fixed inset-0 z-[100] flex items-start justify-center pt-[15vh]"
      overlayClassName="fixed inset-0 bg-black/60 backdrop-blur-sm"
    >
      <div className="w-full max-w-lg bg-card border border-border-strong rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[60vh]">
        {/* Header — changes based on view */}
        {view === "browse" ? (
          <div className="flex items-center gap-2 px-3 py-3 border-b border-border">
            <button
              onClick={() => {
                setView("root");
                setSearch("");
              }}
              className="flex items-center gap-1.5 px-2 py-1.5 rounded-lg text-sm font-medium text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors shrink-0"
            >
              <ArrowLeft size={16} />
              Back
            </button>
            <div className="flex items-center gap-1.5 px-1 text-xs text-foreground-muted font-mono truncate">
              <Folder size={14} className="shrink-0" />
              {browsePath || "Home"}
            </div>
          </div>
        ) : (
          <div className="flex items-center gap-2 px-3 py-3 border-b border-border">
            <Search size={16} className="text-foreground-muted shrink-0 ml-1" />
            <Command.Input
              value={search}
              onValueChange={setSearch}
              placeholder="Search commands..."
              className="flex-1 bg-transparent text-sm text-foreground placeholder:text-foreground-muted outline-none"
            />
          </div>
        )}

        {/* List */}
        <Command.List className="overflow-y-auto p-2 flex-1 min-h-0">
          {browsing && (
            <Command.Loading className="px-3 py-4 text-sm text-foreground-muted">
              Loading...
            </Command.Loading>
          )}

          {view === "root" && (
            <>
              <Command.Empty className="px-3 py-4 text-sm text-foreground-muted">
                No results found.
              </Command.Empty>

              <Command.Group
                heading="Actions"
                className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:text-foreground-muted [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-wider"
              >
                <Command.Item
                  onSelect={() => {
                    setView("browse");
                    setBrowsePath(undefined);
                    setSearch("");
                  }}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-foreground aria-selected:bg-white/8 aria-selected:text-foreground cursor-pointer transition-colors"
                >
                  <FolderPlus size={18} className="text-foreground-secondary shrink-0" />
                  <span className="flex-1">New Project</span>
                  <ChevronRight size={14} className="text-foreground-muted shrink-0" />
                </Command.Item>

                <Command.Item
                  onSelect={handleNewChat}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-foreground aria-selected:bg-white/8 aria-selected:text-foreground cursor-pointer transition-colors"
                >
                  <MessageSquarePlus size={18} className="text-foreground-secondary shrink-0" />
                  <span className="flex-1">New Chat</span>
                </Command.Item>

                <Command.Item
                  onSelect={handleNewTerminal}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-foreground aria-selected:bg-white/8 aria-selected:text-foreground cursor-pointer transition-colors"
                >
                  <TerminalSquare size={18} className="text-foreground-secondary shrink-0" />
                  <span className="flex-1">New Terminal</span>
                </Command.Item>
              </Command.Group>
            </>
          )}

          {view === "browse" && (
            <>
              {/* Up one level */}
              {browse?.parentPath !== null && (
                <Command.Item
                  value=".."
                  onSelect={() => {
                    const parent = browse?.parentPath;
                    setBrowsePath(parent ?? undefined);
                    setSearch("");
                  }}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-foreground-secondary aria-selected:bg-white/8 aria-selected:text-foreground cursor-pointer transition-colors"
                >
                  <ArrowLeft size={16} className="shrink-0" />
                  <span>Go up</span>
                </Command.Item>
              )}

              {/* Current directory — select this folder as the project */}
              <Command.Item
                value={`select:${browsePath ?? "home"}`}
                onSelect={() => {
                  if (browsePath) {
                    handleSelectProject(browsePath);
                  } else if (browse?.path) {
                    handleSelectProject(browse.path);
                  }
                }}
                className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-foreground aria-selected:bg-white/8 aria-selected:text-foreground cursor-pointer transition-colors border-b border-border mb-1"
              >
                <CornerDownLeft size={16} className="text-success shrink-0" />
                <span className="flex-1 font-medium">Select this folder</span>
                <span className="text-xs text-foreground-muted truncate max-w-[200px]">
                  {browsePath || browse?.path || "Home"}
                </span>
              </Command.Item>

              {/* Directory listing */}
              <Command.Group className="p-0">
                {directories.map((dir) => (
                  <Command.Item
                    key={dir.path}
                    value={dir.name}
                    onSelect={() => {
                      setBrowsePath(dir.path);
                      setSearch("");
                    }}
                    className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-foreground-secondary aria-selected:bg-white/8 aria-selected:text-foreground cursor-pointer transition-colors"
                  >
                    <Folder size={16} className="text-foreground-muted shrink-0" />
                    <span className="flex-1 truncate">{dir.name}</span>
                    <ChevronRight
                      size={14}
                      className="text-foreground-muted shrink-0 opacity-0 aria-selected:opacity-100"
                    />
                  </Command.Item>
                ))}
              </Command.Group>

              {browse && directories.length === 0 && !browsing && (
                <div className="px-3 py-4 text-sm text-foreground-muted">
                  No subdirectories found.
                </div>
              )}

              {/* File listing (non-selectable, for context) */}
              {(browse?.entries ?? []).filter((e) => !e.isDir).length > 0 && (
                <Command.Group className="p-0 opacity-50 pointer-events-none">
                  {(browse?.entries ?? [])
                    .filter((e) => !e.isDir)
                    .map((file) => (
                      <Command.Item
                        key={file.path}
                        value={file.name}
                        onSelect={() => {}}
                        className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-foreground-muted"
                      >
                        <File size={14} className="shrink-0" />
                        <span className="flex-1 truncate">{file.name}</span>
                      </Command.Item>
                    ))}
                </Command.Group>
              )}
            </>
          )}
        </Command.List>

        {/* Footer */}
        <div className="flex items-center justify-between px-3 py-2 border-t border-border text-xs text-foreground-muted">
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1">
              <kbd className="font-mono bg-white/5 px-1.5 py-0.5 rounded">↑↓</kbd>
              Navigate
            </span>
            <span className="flex items-center gap-1">
              <kbd className="font-mono bg-white/5 px-1.5 py-0.5 rounded">↵</kbd>
              Select
            </span>
            <span className="flex items-center gap-1">
              <kbd className="font-mono bg-white/5 px-1.5 py-0.5 rounded">esc</kbd>
              Close
            </span>
          </div>
        </div>
      </div>
    </Command.Dialog>
  );
}
