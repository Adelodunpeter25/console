import React, { useEffect, useMemo, useState } from "react";
import { View, Text, TextInput, Pressable } from "react-native";
import { CheckCircle2, XCircle, LoaderCircle } from "lucide-react-native";
import { confirmAlert } from "@/components/common/confirm-dialog";
import {
  useEnvironmentsStore,
  type Environment,
} from "@/stores/useEnvironmentsStore";
import { normalizeBackendUrl } from "@/utils/url";

export type EnvironmentEditorMode = "create" | "edit";

type EditorStatus = "idle" | "testing" | "test-ok" | "test-fail" | "saving";

interface EnvironmentEditorProps {
  mode: EnvironmentEditorMode;
  /** Required in edit mode — the environment being modified. */
  envId?: string;
  /** Called after a successful save or delete so hosts can close their sheet. */
  onDone?: () => void;
}

/**
 * Create/edit form for a single environment. Reusable component — hosted by
 * the environments settings screen and openable from the switcher's "+"
 * row. Tests the currently edited URL (not the stored one) in both modes.
 */
export function EnvironmentEditor({ mode, envId, onDone }: EnvironmentEditorProps) {
  const environments = useEnvironmentsStore((state) => state.environments);
  const activeId = useEnvironmentsStore((state) => state.activeId);
  const addEnvironment = useEnvironmentsStore((state) => state.addEnvironment);
  const updateEnvironment = useEnvironmentsStore((state) => state.updateEnvironment);
  const removeEnvironment = useEnvironmentsStore((state) => state.removeEnvironment);

  const editing: Environment | null =
    mode === "edit" ? environments.find((e) => e.id === envId) ?? null : null;

  const [name, setName] = useState(editing?.name ?? "");
  const [url, setUrl] = useState(editing?.url ?? "");
  const [status, setStatus] = useState<EditorStatus>("idle");

  // Reset to idle whenever any field changes (test results go stale).
  useEffect(() => {
    setStatus((prev) => (prev === "testing" || prev === "saving" ? prev : "idle"));
  }, [name, url]);

  const isActive = Boolean(envId && activeId === envId);
  const canDelete = mode === "edit" && !(isActive && environments.length === 1);

  const testResultIcon = useMemo(() => {
    if (status === "testing") {
      return <LoaderCircle size={14} color="#a1a1aa" />;
    }
    if (status === "test-ok") {
      return <CheckCircle2 size={14} color="#34d399" />;
    }
    if (status === "test-fail") {
      return <XCircle size={14} color="#f87171" />;
    }
    return null;
  }, [status]);

  const handleTest = async () => {
    const normalized = normalizeBackendUrl(url);
    if (!normalized) return;
    setStatus("testing");
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 6000);
      const res = await fetch(`${normalized}/api/projects`, { signal: controller.signal });
      clearTimeout(timeoutId);
      setStatus(res.ok ? "test-ok" : "test-fail");
    } catch {
      setStatus("test-fail");
    }
  };

  const handleSave = () => {
    const normalized = normalizeBackendUrl(url);
    if (!normalized) {
      confirmAlert("Invalid URL", "Backend server endpoint cannot be empty.");
      return;
    }
    if (status !== "test-ok") {
      confirmAlert("Untested environment", "Save without a successful connection test?", [
        { text: "Cancel", style: "cancel" },
        {
          text: "Save anyway",
          onPress: () => commit(normalized),
        },
      ]);
      return;
    }
    commit(normalized);
  };

  const commit = (normalized: string) => {
    setStatus("saving");
    try {
      if (mode === "edit" && envId) {
        updateEnvironment(envId, { name: name.trim() || "Unnamed", url: normalized });
      } else {
        addEnvironment(name.trim() || "Unnamed", normalized);
      }
      onDone?.();
    } finally {
      setStatus("idle");
    }
  };

  const handleDelete = () => {
    if (!envId) return;
    const env = editing;
    confirmAlert(
      "Delete environment",
      `Delete "${env?.name ?? "this environment"}"? This cannot be undone.`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: () => {
            removeEnvironment(envId);
            onDone?.();
          },
        },
      ],
    );
  };

  return (
    <View>
      {/* Name */}
      <Text className="text-xs font-medium text-foreground-secondary mb-1.5">Name</Text>
      <TextInput
        value={name}
        onChangeText={setName}
        placeholder="My server"
        placeholderTextColor="#71717a"
        autoCapitalize="none"
        autoCorrect={false}
        className="bg-card-alt rounded-xl px-3.5 py-2.5 text-sm text-foreground border border-border/50 mb-4"
      />

      {/* URL */}
      <Text className="text-xs font-medium text-foreground-secondary mb-1.5">Backend URL</Text>
      <TextInput
        value={url}
        onChangeText={setUrl}
        placeholder="http://100.82.16.5:3773"
        placeholderTextColor="#71717a"
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="url"
        className="bg-card-alt rounded-xl px-3.5 py-2.5 text-sm text-foreground border border-border/50"
      />

      {/* Test connection */}
      <Pressable
        disabled={!url.trim() || status === "testing"}
        onPress={handleTest}
        style={({ pressed }) => ({ opacity: pressed || !url.trim() ? 0.6 : 1 })}
        className="flex-row items-center justify-center gap-2 rounded-xl border border-border py-2.5 mt-4"
      >
        {testResultIcon}
        <Text className="text-sm font-medium text-foreground">
          {status === "testing" ? "Testing…" : "Test connection"}
        </Text>
      </Pressable>
      {status === "test-ok" ? (
        <Text className="text-xs text-emerald-400 mt-1.5">Connection OK</Text>
      ) : status === "test-fail" ? (
        <Text className="text-xs text-red-400 mt-1.5">Could not reach the backend</Text>
      ) : null}

      {/* Save */}
      <Pressable
        disabled={!url.trim() || status === "saving"}
        onPress={handleSave}
        style={({ pressed }) => ({ opacity: pressed || !url.trim() ? 0.6 : 1 })}
        className="flex-row items-center justify-center gap-2 rounded-xl bg-foreground py-3 mt-3"
      >
        {status === "saving" ? <LoaderCircle size={14} color="#18181b" /> : null}
        <Text className="text-sm font-semibold text-background">
          {mode === "edit" ? "Save changes" : "Save"}
        </Text>
      </Pressable>

      {/* Edit-mode actions */}
      {mode === "edit" ? (
        <View className="flex-row gap-3 mt-3">
          {!isActive ? (
            <Pressable
              onPress={() => {
                if (envId) useEnvironmentsStore.getState().activateEnvironment(envId);
                onDone?.();
              }}
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              className="flex-1 items-center rounded-xl border border-border py-2.5"
            >
              <Text className="text-sm font-medium text-foreground">Set as active</Text>
            </Pressable>
          ) : null}
          <Pressable
            disabled={!canDelete}
            onPress={handleDelete}
            style={({ pressed }) => ({ opacity: pressed || !canDelete ? 0.5 : 1 })}
            className={`flex-1 items-center rounded-xl border border-red-500/30 bg-red-500/5 py-2.5 ${!isActive ? "" : "ml-auto w-full"}`}
          >
            <Text className="text-sm font-medium text-red-400">Delete</Text>
          </Pressable>
        </View>
      ) : null}
    </View>
  );
}
