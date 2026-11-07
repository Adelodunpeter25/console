// The viewer tab contents, ported from console-ui's `viewer/`:
// `file_viewer.rs` (a read-only editor) and `diff_viewer.rs` (a patch).
// The file viewer's comment overlays are dropped until the inspector port.
//
// Data flows through the viewers service; each tab fetches once on mount
// (components run once in nsh) into a signal the JSX reads.

import { signal, Text, View, token } from "nsh";
import { DiffView, Editor } from "nsh/base";
import { viewers, type SessionFileChange } from "../services/viewers";
import { pathLanguage } from "../chat/toolcalls/model";

function ErrorRow(props: { message: string }) {
  return (
    <View class="flex-1 items-center justify-center p-4">
      <Text class="text-[12px] text-muted-foreground">{props.message}</Text>
    </View>
  );
}

/// A tab's content while its data loads.
function LoadingRow() {
  return (
    <View class="flex-1 items-center justify-center">
      <Text class="text-[12px] text-muted-foreground">Loading…</Text>
    </View>
  );
}

function baseName(path: string): string {
  const at = path.lastIndexOf("/");
  return at >= 0 ? path.slice(at + 1) : path;
}

/// The `file` tab: the file at HEAD in a read-only editor
/// (desktop `file_viewer.rs`, without its comment overlays).
export function FileViewerTab(props: { baseUrl: string; path: string }) {
  const result = signal<{ content: string } | null>(null);
  const error = signal<string | null>(null);
  void viewers
    .fileContent(props.baseUrl, props.path)
    .then((file) => result.set({ content: file.content }))
    .catch((err) => error.set(err instanceof Error ? err.message : String(err)));
  return (
    <View class="flex-1 flex-col overflow-hidden">
      {(() => {
        if (error() != null) return <ErrorRow message={error()!} />;
        if (result() == null) return <LoadingRow />;
        return (
          <Editor
            value={result()!.content}
            readOnly={true}
            language={pathLanguage(props.path)}
            fontSize={11}
            class="flex-1"
          />
        );
      })()}
    </View>
  );
}

/// The `diff` tab: one file's diff against HEAD (desktop `diff_viewer.rs`).
/// `repoPath` is the project's checkout; without it the server refuses the
/// call, so the tab explains what is missing instead.
export function DiffViewerTab(props: {
  baseUrl: string;
  repoPath: string | null;
  path: string;
}) {
  const patch = signal<string | null>(null);
  const error = signal<string | null>(null);
  void (props.repoPath == null
    ? Promise.reject(new Error("This tab's project has no path on disk"))
    : viewers
        .fileDiff(props.baseUrl, props.repoPath, props.path)
        .then((d) => patch.set(d.diff))
  )
    .catch((err: unknown) => error.set(err instanceof Error ? err.message : String(err)));
  return (
    <View class="flex-1 flex-col overflow-hidden">
      {(() => {
        if (error() != null) return <ErrorRow message={error()!} />;
        if (patch() == null) return <LoadingRow />;
        return (
          <DiffView
            patch={patch()}
            language={pathLanguage(props.path)}
            layout="unified"
            indicators="bars"
            inline="word"
            fontSize={11}
            lineHeight={18}
            class="flex-1"
          />
        );
      })()}
    </View>
  );
}

/// One file row of the review tab: name, counts, the reviewed toggle, and
/// the file's diff expanded underneath (desktop `review_tab.rs`; its
/// `REVIEW_DIFF_MAX_HEIGHT` cap, natively by DiffView's scroll).
function ReviewFileRow(props: {
  baseUrl: string;
  sessionId: string;
  change: SessionFileChange;
}) {
  const change = props.change;
  const expanded = signal(change.diffText != null);
  const patch = signal<string | null>(change.diffText);
  const reviewed = signal(change.reviewed);
  const load = () => {
    if (patch() == null) {
      void viewers
        .changeDiff(props.baseUrl, props.sessionId, change.path, change.turnIndex)
        .then((diff) => patch.set(diff))
        .catch((err) => patch.set(`// ${err instanceof Error ? err.message : String(err)}`));
    }
  };
  return (
    <View class="flex-none flex-col border-b border-border">
      <View class="flex-row items-center gap-2 px-3 py-1.5">
        <View
          class="flex-1 flex-row items-center gap-2 hover:bg-accent"
          role="button"
          accessibilityLabel={`Toggle diff for ${baseName(change.path)}`}
          onPress={() => {
            expanded.set(!expanded());
            load();
          }}
        >
          <Text class="flex-1 truncate text-[12px] text-foreground">{change.path}</Text>
          <View class="flex-row items-center gap-1 text-[11px] font-semibold">
            <Text style={{ color: token("primary") }}>{`+${change.additions}`}</Text>
            <Text style={{ color: token("destructive") }}>{`-${change.deletions}`}</Text>
          </View>
        </View>
        <View
          class={
            "rounded border px-1.5 py-[1px] " +
            (reviewed() ? "border-primary" : "border-border")
          }
          role="button"
          accessibilityLabel={
            reviewed() ? `Mark ${baseName(change.path)} unreviewed` : `Mark ${baseName(change.path)} reviewed`
          }
          onPress={() => {
            const next = !reviewed();
            reviewed.set(next);
            void viewers
              .setChangeReviewed(
                props.baseUrl,
                props.sessionId,
                change.path,
                change.turnIndex,
                next,
              )
              .catch(() => reviewed.set(!next));
          }}
        >
          <Text
            class="text-[10px]"
            style={{ color: token(reviewed() ? "primary" : "muted-foreground") }}
          >
            {reviewed() ? "Reviewed" : "Review"}
          </Text>
        </View>
      </View>
      {(() => {
        if (!expanded() || patch() == null) return null;
        return (
          <View class="max-h-[800px] flex-none px-3 pb-2">
            <DiffView
              patch={patch()}
              language={pathLanguage(change.path)}
              layout="unified"
              indicators="bars"
              inline="word"
              fontSize={11}
              lineHeight={18}
            />
          </View>
        );
      })()}
    </View>
  );
}

/// The `changesReview` tab: every file the session changed, stacked, with
/// per-file expand and mark-reviewed (desktop `review_tab.rs`).
export function ReviewTab(props: { baseUrl: string; sessionId: string }) {
  const changes = signal<SessionFileChange[] | null>(null);
  const error = signal<string | null>(null);
  void viewers
    .sessionChanges(props.baseUrl, props.sessionId, null)
    .then((rows) => changes.set(rows))
    .catch((err) => error.set(err instanceof Error ? err.message : String(err)));
  return (
    <View class="flex-1 flex-col overflow-hidden">
      {(() => {
        if (error() != null) return <ErrorRow message={error()!} />;
        if (changes() == null) return <LoadingRow />;
        if (changes()!.length === 0)
          return <ErrorRow message="No file changes in this session" />;
        return changes()!.map((change) => (
          <ReviewFileRow
            baseUrl={props.baseUrl}
            sessionId={props.sessionId}
            change={change}
          />
        ));
      })()}
    </View>
  );
}
