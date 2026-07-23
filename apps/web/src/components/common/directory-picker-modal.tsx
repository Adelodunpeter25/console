import React, { useState } from "react";
import { useFsBrowse } from "@console/api";
import { Folder, FolderPlus, ChevronRight, HardDrive, Check, X } from "lucide-react";

export interface DirectoryPickerModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSelect: (path: string) => void;
}

export const DirectoryPickerModal: React.FC<DirectoryPickerModalProps> = ({
  isOpen,
  onClose,
  onSelect,
}) => {
  const [currentPath, setCurrentPath] = useState<string | undefined>(undefined);
  const { data, isLoading } = useFsBrowse(currentPath);

  if (!isOpen) return null;

  const entries = data?.entries?.filter((e) => e.isDir) || [];

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-xs flex items-center justify-center p-4">
      <div className="bg-card border border-border/80 rounded-xl w-full max-w-lg shadow-2xl flex flex-col max-h-[80vh]">
        {/* Modal Header */}
        <div className="p-3.5 border-b border-border/40 flex items-center justify-between">
          <div className="flex items-center gap-2 text-foreground font-medium text-sm">
            <FolderPlus size={16} className="text-primary" />
            <span>Select Local Workspace Directory</span>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-md hover:bg-accent text-muted-foreground hover:text-foreground cursor-pointer"
          >
            <X size={16} />
          </button>
        </div>

        {/* Current Path Bar */}
        <div className="px-3.5 py-2 bg-background/50 border-b border-border/30 flex items-center gap-2 text-xs font-mono text-muted-foreground overflow-x-auto">
          <HardDrive size={13} className="shrink-0 text-primary/70" />
          <span className="truncate">{data?.currentPath || "System Root"}</span>
        </div>

        {/* Directory Navigation List */}
        <div className="flex-1 overflow-y-auto p-2 min-h-[240px]">
          {data?.parentPath && (
            <button
              onClick={() => setCurrentPath(data.parentPath || undefined)}
              className="w-full text-left px-2.5 py-1.5 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-accent/50 flex items-center gap-2 font-mono"
            >
              <span>.. (Parent Directory)</span>
            </button>
          )}

          {isLoading ? (
            <div className="p-6 text-center text-xs text-muted-foreground">Loading directories...</div>
          ) : entries.length === 0 ? (
            <div className="p-6 text-center text-xs text-muted-foreground">No subdirectories found</div>
          ) : (
            entries.map((entry) => (
              <button
                key={entry.path}
                onClick={() => setCurrentPath(entry.path)}
                className="w-full text-left px-2.5 py-1.5 rounded-md text-xs text-foreground hover:bg-accent flex items-center justify-between group"
              >
                <div className="flex items-center gap-2 truncate">
                  <Folder size={14} className="text-primary/80 shrink-0" />
                  <span className="truncate">{entry.name}</span>
                </div>
                <ChevronRight size={14} className="text-muted-foreground opacity-0 group-hover:opacity-100" />
              </button>
            ))
          )}
        </div>

        {/* Modal Action Footer */}
        <div className="p-3 border-t border-border/40 flex items-center justify-between bg-card/50">
          <span className="text-[11px] text-muted-foreground truncate max-w-[260px]">
            {data?.currentPath}
          </span>
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-xs rounded-md border border-border text-muted-foreground hover:bg-accent"
            >
              Cancel
            </button>
            <button
              disabled={!data?.currentPath}
              onClick={() => {
                if (data?.currentPath) {
                  onSelect(data.currentPath);
                  onClose();
                }
              }}
              className="px-3 py-1.5 text-xs rounded-md bg-primary text-primary-foreground font-medium flex items-center gap-1.5 disabled:opacity-50"
            >
              <Check size={13} /> Select Folder
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
