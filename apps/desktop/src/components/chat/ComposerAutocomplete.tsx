import React from "react";
import { FileText, Command, Folder, Loader2 } from "lucide-react";
import type { FileSearchResult, SlashCommandInfo } from "@console/types";
import { api } from "../../lib/api";

export interface SlashSuggestion {
  kind: "slash";
  name: string;
  description: string;
}

export interface FileSuggestion {
  kind: "file";
  item: FileSearchResult;
}

export type Suggestion = SlashSuggestion | FileSuggestion;

interface ComposerAutocompleteProps {
  /** Current text in the composer. */
  value: string;
  /** Session id — scopes slash commands and the FFF file search root. */
  sessionId: string | null;
  /** Called with the full new composer value after a suggestion is picked. */
  onPick: (value: string) => void;
  /** The composer textarea, for caret + keyboard interception. */
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
}

interface ActiveTrigger {
  kind: "slash" | "file";
  /** Index in the value where the trigger char starts. */
  start: number;
  /** The typed text after the trigger char. */
  query: string;
}

/**
 * Detect an active autocomplete trigger at the caret:
 *  - `/` at the start of a line → slash command
 *  - `@` preceded by whitespace/start → file reference
 */
function getActiveTrigger(value: string, caret: number): ActiveTrigger | null {
  const before = value.slice(0, caret);

  // Slash trigger: `/` must be the first non-newline char on the line.
  const lineStart = before.lastIndexOf("\n") + 1;
  if (before[lineStart] === "/") {
    const after = value.slice(lineStart + 1, caret);
    if (/^[\w:-]*$/.test(after)) {
      return { kind: "slash", start: lineStart, query: after };
    }
  }

  // @ trigger: must follow whitespace or line start (so emails don't match).
  const atIdx = before.lastIndexOf("@");
  if (atIdx >= 0) {
    const prev = atIdx > 0 ? before[atIdx - 1] : undefined;
    const after = value.slice(atIdx + 1, caret);
    if ((prev === undefined || /\s/.test(prev)) && /^[\w./-]*$/.test(after)) {
      return { kind: "file", start: atIdx, query: after };
    }
  }

  return null;
}

/**
 * Slash-command and @-file autocomplete for the composer. Listens for `/`
 * and `@` triggers on the textarea, queries the server (slash command list /
 * FFF fuzzy file search), and lets the user pick with arrows + Enter/Tab or
 * click. Keyboard handling is attached natively on the textarea so it runs
 * before the composer's Enter-to-send handler.
 */
export function ComposerAutocomplete({
  value,
  sessionId,
  onPick,
  textareaRef,
}: ComposerAutocompleteProps) {
  const [slashCommands, setSlashCommands] = React.useState<SlashCommandInfo[]>([]);
  const [files, setFiles] = React.useState<FileSearchResult[]>([]);
  const [trigger, setTrigger] = React.useState<ActiveTrigger | null>(null);
  const [loading, setLoading] = React.useState(false);
  const [selectedIndex, setSelectedIndex] = React.useState(0);
  const requestSeq = React.useRef(0);

  // Stable refs so the native keydown handler never sees stale closures.
  const valueRef = React.useRef(value);
  valueRef.current = value;
  const triggerRef = React.useRef(trigger);
  triggerRef.current = trigger;
  const selectedIndexRef = React.useRef(selectedIndex);
  selectedIndexRef.current = selectedIndex;

  // The currently-rendered suggestion list, mirrored in a ref for key handling.
  const rendered: Suggestion[] = React.useMemo(() => {
    if (!trigger) return [];
    if (trigger.kind === "slash") {
      const q = trigger.query.toLowerCase();
      return slashCommands
        .filter(
          (c) => c.name.toLowerCase().startsWith(q) || c.description.toLowerCase().includes(q),
        )
        .map((c) => ({ kind: "slash" as const, name: c.name, description: c.description }));
    }
    return files.map((f) => ({ kind: "file" as const, item: f }));
  }, [trigger, slashCommands, files]);

  const suggestionsRef = React.useRef<Suggestion[]>([]);
  suggestionsRef.current = rendered;

  const pickAt = React.useCallback(
    (index: number) => {
      const trig = triggerRef.current;
      const list = suggestionsRef.current;
      const s = list[index];
      if (!trig || !s) return;
      const v = valueRef.current;
      const before = v.slice(0, trig.start);
      const after = v.slice(trig.start + 1 + trig.query.length);
      const text = s.kind === "slash" ? `/${s.name} ` : `@${s.item.relativePath} `;
      onPick(`${before}${text}${after}`);
      setTrigger(null);
    },
    [onPick],
  );

  // Load slash commands once per session.
  React.useEffect(() => {
    let cancelled = false;
    if (sessionId) {
      api
        .listSlashCommands(sessionId)
        .then((cmds) => !cancelled && setSlashCommands(cmds))
        .catch(() => {});
    }
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  // Track the caret and intercept navigation keys while a trigger is active.
  React.useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;

    const sync = () => {
      setTrigger(getActiveTrigger(valueRef.current, el.selectionStart ?? el.value.length));
    };

    const onKeyDown = (e: KeyboardEvent) => {
      const trig = triggerRef.current;
      const list = suggestionsRef.current;
      if (!trig || list.length === 0) return;
      if (["ArrowDown", "ArrowUp", "Enter", "Tab", "Escape"].includes(e.key)) {
        e.preventDefault();
        e.stopPropagation();
        if (e.key === "ArrowDown") {
          setSelectedIndex((i) => (i + 1) % list.length);
        } else if (e.key === "ArrowUp") {
          setSelectedIndex((i) => (i - 1 + list.length) % list.length);
        } else if (e.key === "Enter" || e.key === "Tab") {
          pickAt(selectedIndexRef.current);
        } else if (e.key === "Escape") {
          setTrigger(null);
        }
      }
    };

    el.addEventListener("input", sync);
    el.addEventListener("keyup", sync);
    el.addEventListener("click", sync);
    el.addEventListener("keydown", onKeyDown);
    return () => {
      el.removeEventListener("input", sync);
      el.removeEventListener("keyup", sync);
      el.removeEventListener("click", sync);
      el.removeEventListener("keydown", onKeyDown);
    };
  }, [textareaRef, pickAt]);

  // Fetch suggestions when the trigger/query changes.
  React.useEffect(() => {
    if (!trigger) {
      setFiles([]);
      setSelectedIndex(0);
      setLoading(false);
      return;
    }
    const seq = ++requestSeq.current;

    if (trigger.kind === "slash") {
      setLoading(false);
      setSelectedIndex(0);
    } else if (trigger.kind === "file" && sessionId) {
      setLoading(true);
      api
        .searchFiles(sessionId, trigger.query)
        .then((res) => {
          if (seq === requestSeq.current) {
            setFiles(res.items);
            setSelectedIndex(0);
          }
        })
        .catch(() => {})
        .finally(() => {
          if (seq === requestSeq.current) setLoading(false);
        });
    }
  }, [trigger, sessionId]);

  if (!trigger || (rendered.length === 0 && !loading)) return null;

  return (
    <div className="absolute bottom-full left-0 right-0 mb-1 z-50">
      <div className="max-h-60 overflow-y-auto rounded-xl border border-border bg-card shadow-xl">
        {loading && trigger.kind === "file" && rendered.length === 0 && (
          <div className="flex items-center gap-2 px-3 py-2 text-xs text-foreground-muted">
            <Loader2 size={12} className="animate-spin" />
            Searching…
          </div>
        )}
        {rendered.map((s, i) => {
          const active = i === selectedIndex;
          if (s.kind === "slash") {
            return (
              <button
                key={`/ ${s.name}`}
                onClick={() => pickAt(i)}
                onMouseEnter={() => setSelectedIndex(i)}
                className={`w-full flex items-start gap-2 px-3 py-2 text-left ${
                  active ? "bg-white/5" : ""
                }`}
              >
                <Command size={13} className="shrink-0 mt-0.5 text-foreground-muted" />
                <span className="min-w-0">
                  <span className="block text-xs font-medium text-foreground">/{s.name}</span>
                  <span className="block text-[11px] text-foreground-muted truncate">
                    {s.description}
                  </span>
                </span>
              </button>
            );
          }
          return (
            <button
              key={`@ ${s.item.relativePath}`}
              onClick={() => pickAt(i)}
              onMouseEnter={() => setSelectedIndex(i)}
              className={`w-full flex items-center gap-2 px-3 py-2 text-left ${
                active ? "bg-white/5" : ""
              }`}
            >
              {s.item.isDir ? (
                <Folder size={13} className="shrink-0 text-foreground-muted" />
              ) : (
                <FileText size={13} className="shrink-0 text-foreground-muted" />
              )}
              <span className="text-xs font-mono text-foreground truncate">
                {s.item.relativePath}
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
