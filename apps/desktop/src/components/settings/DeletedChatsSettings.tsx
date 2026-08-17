import React from "react";
import { AlertTriangle, Loader2, RotateCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useProjectStore } from "../../store/useProjectStore";
import { api } from "../../lib/api";
import { basename } from "../../utils/format";
import { formatRelativeTime } from "../../utils/time";
import { GlassSurface } from "../common/GlassSurface";

/** Settings panel for recovering or permanently removing soft-deleted chats. */
export function DeletedChatsSettings() {
  const deletedSessions = useProjectStore((state) => state.deletedSessions);
  const loading = useProjectStore((state) => state.deletedSessionsLoading);
  const loadDeletedSessions = useProjectStore((state) => state.loadDeletedSessions);
  const restoreSession = useProjectStore((state) => state.restoreSession);
  const permanentlyDeleteSession = useProjectStore((state) => state.permanentlyDeleteSession);
  const [busyId, setBusyId] = React.useState<string | null>(null);

  React.useEffect(() => {
    void loadDeletedSessions();
  }, [loadDeletedSessions]);

  const handleRestore = async (id: string) => {
    setBusyId(id);
    try {
      await restoreSession(id);
      toast.success("Chat restored.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to restore chat.");
    } finally {
      setBusyId(null);
    }
  };

  const handlePermanentDelete = async (id: string, title: string) => {
    const confirmed = await api.confirmDialog(
      "Delete Chat Permanently",
      `"${title}" and its message history will be permanently deleted. This cannot be undone.`,
    );
    if (!confirmed) return;

    setBusyId(id);
    try {
      await permanentlyDeleteSession(id);
      toast.success("Chat permanently deleted.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to permanently delete chat.");
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto bg-screen px-5 py-5">
      <div className="mb-5">
        <h2 className="text-base font-semibold tracking-tight text-foreground">Deleted Chats</h2>
        <p className="mt-0.5 text-xs text-foreground-secondary">
          Restore deleted chats or permanently remove them and their message history.
        </p>
      </div>

      <GlassSurface className="mb-4 border-warning/20 bg-warning-muted/30 p-4">
        <div className="flex items-start gap-2">
          <AlertTriangle size={15} className="mt-0.5 shrink-0 text-warning" />
          <p className="text-xs leading-5 text-foreground-secondary">
            Chats are initially soft-deleted and can be restored. Permanent deletion cannot be
            undone.
          </p>
        </div>
      </GlassSurface>

      {loading ? (
        <div className="flex items-center gap-2 text-xs text-foreground-muted">
          <Loader2 size={14} className="animate-spin" /> Loading deleted chats...
        </div>
      ) : deletedSessions.length === 0 ? (
        <GlassSurface className="p-8 text-center">
          <Trash2 size={22} className="mx-auto mb-2 text-foreground-muted" />
          <p className="text-sm text-foreground-secondary">No deleted chats</p>
          <p className="mt-1 text-xs text-foreground-muted">
            Chats you delete will appear here.
          </p>
        </GlassSurface>
      ) : (
        <div className="space-y-2">
          {deletedSessions.map((session) => {
            const busy = busyId === session.id;
            const title = session.title || "Untitled Chat";
            return (
              <GlassSurface key={session.id} className="p-3">
                <div className="flex items-center gap-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-foreground">{title}</p>
                    <p className="mt-0.5 truncate text-[11px] text-foreground-muted">
                      {basename(session.cwd)} · Deleted {formatRelativeTime(session.deletedAt ?? session.updatedAt, true)}
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => void handleRestore(session.id)}
                      className="flex items-center gap-1 rounded-md border border-border px-2.5 py-1.5 text-[11px] text-foreground-secondary transition-colors hover:bg-white/[0.08] hover:text-foreground disabled:opacity-50"
                    >
                      {busy ? <Loader2 size={12} className="animate-spin" /> : <RotateCcw size={12} />}
                      Restore
                    </button>
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => void handlePermanentDelete(session.id, title)}
                      className="flex items-center gap-1 rounded-md border border-danger/30 px-2.5 py-1.5 text-[11px] text-danger transition-colors hover:bg-danger-muted disabled:opacity-50"
                    >
                      <Trash2 size={12} /> Delete permanently
                    </button>
                  </div>
                </div>
              </GlassSurface>
            );
          })}
        </div>
      )}
    </div>
  );
}
