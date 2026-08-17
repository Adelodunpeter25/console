import React from "react";
import { Command } from "cmdk";
import { FileText, LoaderCircle, Search } from "lucide-react";
import type { FileSearchResult } from "@console/types";
import { api } from "../../lib/api";
import { useAppStore } from "../../store/useAppStore";
import { useProjectStore } from "../../store/useProjectStore";
import { useWorkspaceStore } from "../../layout/useWorkspaceStore";

interface FileSearchPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Quick-open palette for files in the active project. */
export function FileSearchPalette({ open, onOpenChange }: FileSearchPaletteProps) {
  const [query, setQuery] = React.useState("");
  const [results, setResults] = React.useState<FileSearchResult[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const requestId = React.useRef(0);

  const selectedProjectId = useAppStore((state) => state.selectedProjectId);
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const projects = useProjectStore((state) => state.projects);
  const sessions = useProjectStore((state) => state.sessions);
  const openFileTab = useWorkspaceStore((state) => state.openFileTab);

  const activeProject = projects.find((project) => project.id === selectedProjectId);
  const activeSession = sessions.find((session) => session.id === selectedSessionId);
  const projectId = selectedProjectId ?? activeSession?.projectId ?? "";
  const searchRoot = activeProject?.path;

  React.useEffect(() => {
    if (!open) return;

    const trimmedQuery = query.trim();
    if (!trimmedQuery) {
      setResults([]);
      setError(null);
      setLoading(false);
      return;
    }

    const currentRequestId = ++requestId.current;
    const timer = window.setTimeout(() => {
      setLoading(true);
      setError(null);
      api
        .searchFiles(selectedSessionId ?? "", trimmedQuery, searchRoot)
        .then((response) => {
          if (currentRequestId !== requestId.current) return;
          setResults(response.items.filter((item) => !item.isDir));
        })
        .catch((err: unknown) => {
          if (currentRequestId !== requestId.current) return;
          setResults([]);
          setError(err instanceof Error ? err.message : "Unable to search files");
        })
        .finally(() => {
          if (currentRequestId === requestId.current) setLoading(false);
        });
    }, 150);

    return () => window.clearTimeout(timer);
  }, [open, query, searchRoot, selectedSessionId]);

  React.useEffect(() => {
    if (!open) {
      setQuery("");
      setResults([]);
      setError(null);
      setLoading(false);
      requestId.current += 1;
    }
  }, [open]);

  const handleSelect = (file: FileSearchResult) => {
    if (!projectId) return;
    openFileTab({
      type: "file",
      projectId,
      path: file.absolutePath,
      title: file.relativePath,
    });
    onOpenChange(false);
  };

  return (
    <Command.Dialog
      open={open}
      onOpenChange={onOpenChange}
      label="Search files"
      shouldFilter={false}
      className="fixed inset-0 z-[101] flex items-start justify-center pt-[15vh]"
      overlayClassName="fixed inset-0 bg-black/60 backdrop-blur-sm"
    >
      <div className="w-full max-w-lg bg-card border border-border-strong rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[60vh]">
        <div className="flex items-center gap-2 px-3 py-3 border-b border-border">
          <Search size={16} className="text-foreground-muted shrink-0 ml-1" />
          <Command.Input
            autoFocus
            value={query}
            onValueChange={setQuery}
            placeholder="Search files..."
            className="flex-1 bg-transparent text-sm text-foreground placeholder:text-foreground-muted outline-none"
          />
          <kbd className="font-mono text-[10px] text-foreground-muted bg-white/5 px-1.5 py-0.5 rounded">
            esc
          </kbd>
        </div>

        <Command.List className="overflow-y-auto p-2 flex-1 min-h-0">
          {loading && (
            <div className="flex items-center justify-center gap-2 px-3 py-6 text-sm text-foreground-muted">
              <LoaderCircle size={15} className="animate-spin" />
              Searching files...
            </div>
          )}

          {!loading && error && <div className="px-3 py-6 text-sm text-danger">{error}</div>}

          {!loading && !error && !query.trim() && (
            <div className="px-3 py-6 text-sm text-foreground-muted text-center">
              Type a filename or path to search the active project.
            </div>
          )}

          {!loading && !error && query.trim() && (
            <Command.Empty className="px-3 py-6 text-sm text-foreground-muted text-center">
              No files found.
            </Command.Empty>
          )}

          {!loading && !error && results.length > 0 && (
            <Command.Group
              heading="Files"
              className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:text-foreground-muted [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-wider"
            >
              {results.map((file) => (
                <Command.Item
                  key={file.absolutePath}
                  value={file.relativePath}
                  onSelect={() => handleSelect(file)}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm text-foreground-secondary aria-selected:bg-white/8 aria-selected:text-foreground cursor-pointer transition-colors"
                >
                  <FileText size={16} className="text-foreground-muted shrink-0" />
                  <span className="truncate">{file.relativePath}</span>
                </Command.Item>
              ))}
            </Command.Group>
          )}
        </Command.List>

        <div className="flex items-center gap-3 px-3 py-2 border-t border-border text-xs text-foreground-muted">
          <span className="flex items-center gap-1">
            <kbd className="font-mono bg-white/5 px-1.5 py-0.5 rounded">↑↓</kbd>
            Navigate
          </span>
          <span className="flex items-center gap-1">
            <kbd className="font-mono bg-white/5 px-1.5 py-0.5 rounded">↵</kbd>
            Open
          </span>
        </div>
      </div>
    </Command.Dialog>
  );
}
