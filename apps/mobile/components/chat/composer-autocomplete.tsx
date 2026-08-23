import React, { useEffect, useMemo, useRef, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView } from "react-native";
import { Command, FileText, Folder, Loader2 } from "lucide-react-native";
import type { FileSearchResult, SlashCommandInfo } from "@console/types";
import { assistService } from "@console/api";
import { theme } from "../../styles/theme";

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
  readonly value: string;
  readonly selectionStart: number;
  readonly sessionId: string | null | undefined;
  readonly onPick: (value: string) => void;
}

interface ActiveTrigger {
  kind: "slash" | "file";
  start: number;
  query: string;
}

function getActiveTrigger(value: string, caret: number): ActiveTrigger | null {
  const before = value.slice(0, caret);

  const lineStart = before.lastIndexOf("\n") + 1;
  if (before[lineStart] === "/") {
    const after = value.slice(lineStart + 1, caret);
    if (/^[\w:-]*$/.test(after)) {
      return { kind: "slash", start: lineStart, query: after };
    }
  }

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

export function ComposerAutocomplete({ value, selectionStart, sessionId, onPick }: ComposerAutocompleteProps) {
  const [slashCommands, setSlashCommands] = useState<SlashCommandInfo[]>([]);
  const [files, setFiles] = useState<FileSearchResult[]>([]);
  const [trigger, setTrigger] = useState<ActiveTrigger | null>(null);
  const [loading, setLoading] = useState(false);
  const requestSeq = useRef(0);

  const valueRef = useRef(value);
  valueRef.current = value;

  // Sync trigger when value or caret changes
  useEffect(() => {
    setTrigger(getActiveTrigger(value, selectionStart));
  }, [value, selectionStart]);

  // Load slash commands once per session
  useEffect(() => {
    let cancelled = false;
    if (sessionId) {
      assistService
        .listSlashCommands(sessionId)
        .then((cmds) => {
          if (!cancelled) setSlashCommands(cmds);
        })
        .catch(() => {});
    } else {
      setSlashCommands([]);
    }
    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  // Fetch file search results when trigger is file
  useEffect(() => {
    if (!trigger || trigger.kind !== "file" || !sessionId) {
      setFiles([]);
      setLoading(false);
      return;
    }
    const seq = ++requestSeq.current;
    // Empty query still searches (shows recent files) — mirror desktop behavior
    setLoading(true);
    assistService
      .searchFiles(sessionId, trigger.query)
      .then((res) => {
        if (seq === requestSeq.current) {
          setFiles(res.items.slice(0, 20));
        }
      })
      .catch(() => {})
      .finally(() => {
        if (seq === requestSeq.current) setLoading(false);
      });
  }, [trigger?.kind, (trigger as ActiveTrigger | null)?.query, sessionId]);

  const rendered: Suggestion[] = useMemo(() => {
    if (!trigger) return [];
    if (trigger.kind === "slash") {
      const q = trigger.query.toLowerCase();
      return slashCommands
        .filter((c) => c.name.toLowerCase().startsWith(q) || c.description.toLowerCase().includes(q))
        .slice(0, 20)
        .map((c) => ({ kind: "slash" as const, name: c.name, description: c.description }));
    }
    return files.map((f) => ({ kind: "file" as const, item: f }));
  }, [trigger, slashCommands, files]);

  const pickAt = (index: number) => {
    if (!trigger) return;
    const s = rendered[index];
    if (!s) return;
    const before = value.slice(0, trigger.start);
    const after = value.slice(trigger.start + 1 + trigger.query.length);
    const text = s.kind === "slash" ? `/${s.name} ` : `@${s.item.relativePath} `;
    onPick(`${before}${text}${after}`);
  };

  if (!trigger || (rendered.length === 0 && !loading)) return null;

  return (
    <View className="mx-2 mb-2 rounded-xl border border-border bg-card shadow-xl overflow-hidden">
      <ScrollView
        style={{ maxHeight: 220 }}
        keyboardShouldPersistTaps="handled"
        showsVerticalScrollIndicator={false}
        contentContainerStyle={{ paddingVertical: 4 }}
      >
        {loading && trigger.kind === "file" && rendered.length === 0 ? (
          <View className="flex-row items-center gap-2 px-3 py-2.5">
            <Loader2 size={12} color={theme.colors.text.muted} />
            <Text className="text-xs text-foreground-muted">Searching…</Text>
          </View>
        ) : null}

        {rendered.map((s, i) => {
          if (s.kind === "slash") {
            return (
              <Pressable
                key={`/${s.name}`}
                onPress={() => pickAt(i)}
                className="flex-row items-start gap-2 px-3 py-2.5 active:bg-white/5"
              >
                <Command size={13} color={theme.colors.text.muted} style={{ marginTop: 2 }} />
                <View className="min-w-0 flex-1">
                  <Text className="text-xs font-medium text-foreground">/{s.name}</Text>
                  <Text className="text-[11px] text-foreground-muted mt-0.5" numberOfLines={1}>
                    {s.description}
                  </Text>
                </View>
              </Pressable>
            );
          }
          return (
            <Pressable
              key={`@${s.item.relativePath}`}
              onPress={() => pickAt(i)}
              className="flex-row items-center gap-2 px-3 py-2.5 active:bg-white/5"
            >
              {s.item.isDir ? (
                <Folder size={13} color={theme.colors.text.muted} />
              ) : (
                <FileText size={13} color={theme.colors.text.muted} />
              )}
              <Text className="text-xs font-mono text-foreground flex-1" numberOfLines={1}>
                {s.item.relativePath}
              </Text>
            </Pressable>
          );
        })}

        {!loading && rendered.length === 0 && trigger ? (
          <View className="px-3 py-2.5">
            <Text className="text-xs text-foreground-muted">
              {trigger.kind === "slash" ? "No commands found" : "No files found"}
            </Text>
          </View>
        ) : null}
      </ScrollView>
    </View>
  );
}
