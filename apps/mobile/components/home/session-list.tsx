import React from "react";
import { View, Text, Pressable } from "react-native";
import { Plus, MessageSquare, Image as ImageIcon } from "lucide-react-native";
import { HugeiconsIcon } from "@hugeicons/react-native";
import { Folder02Icon } from "@hugeicons/core-free-icons";
import type { SessionHeader } from "@console/types";
import type { GroupedProjectSection } from "@/hooks/useHomeSessions";
import { formatRelativeTime } from "@/utils/time";
import { theme } from "@/styles/theme";
import { confirmAlert } from "@/components/common/confirm-dialog";
import { EmptyState } from "@/components/common/empty-state";
import { SessionListSkeleton } from "@/components/common/skeleton";
import { useChatStore } from "@/stores";
import { draftPreview, isDraftSession } from "@/stores/chat/draft";

function getStatusStyle(status?: string): {
  label: string;
  color: string;
  bgColor: string;
} {
  switch (status) {
    case "working":
      return {
        label: "Running",
        color: theme.colors.status.running,
        bgColor: theme.colors.status.runningBg,
      };
    case "done":
      return {
        label: "Ready",
        color: theme.colors.status.ready,
        bgColor: theme.colors.status.readyBg,
      };
    case "needs_attention":
      return {
        label: "Attention",
        color: theme.colors.status.attention,
        bgColor: theme.colors.status.attentionBg,
      };
    default:
      return {
        label: "Idle",
        color: theme.colors.status.idle,
        bgColor: theme.colors.status.idleBg,
      };
  }
}

function shortRelativeTime(dateInput?: number): string {
  const full = formatRelativeTime(dateInput);
  return full.replace(" ago", "").replace("just now", "now");
}

interface SessionListProps {
  sections: GroupedProjectSection[];
  searchQuery: string;
  isLoading: boolean;
  isCreatingSession?: boolean;
  openSession: (id: string) => void;
  prefetchSession?: (id: string) => void;
  composeSession: (projectId?: string | null) => Promise<any>;
  onLongPressSession: (session: SessionHeader) => void;
  getProjectNameForSession: (session: SessionHeader) => string;
  getBranchForSession: (session: SessionHeader) => string | undefined;
}

export function SessionList({
  sections,
  searchQuery,
  isLoading,
  isCreatingSession,
  openSession,
  prefetchSession,
  composeSession,
  onLongPressSession,
  getProjectNameForSession,
  getBranchForSession,
}: SessionListProps) {
  const chatSessions = useChatStore((s) => s.sessions);
  if (isLoading && sections.length === 0) {
    return <SessionListSkeleton />;
  }

  if (sections.length === 0) {
    return (
      <EmptyState
        icon={<MessageSquare size={32} color={theme.colors.text.muted} />}
        title={searchQuery ? "No matching sessions" : "No chat sessions"}
        description={
          searchQuery
            ? `No chats found matching "${searchQuery}".`
            : "Start a new chat or select a project folder to get started."
        }
      />
    );
  }

  return (
    <>
      {sections.map((section) => (
        <View key={section.projectId ?? "no-project"} className="mb-6">
          {/* Section header */}
          <View className="flex-row items-center justify-between mb-2 px-1">
            <View className="flex-row items-center gap-2">
              <HugeiconsIcon icon={Folder02Icon} size={14} color={theme.colors.text.muted} />
              <Text className="text-xs font-semibold text-foreground-secondary tracking-wide">
                {section.projectName}
              </Text>
            </View>
            <Pressable
              className="w-6 h-6 rounded-md items-center justify-center bg-card-alt/80 border border-border/40"
              style={({ pressed }) => ({ opacity: pressed ? 0.6 : 1 })}
              hitSlop={8}
              onPress={async () => {
                try {
                  await composeSession(section.projectId);
                } catch {
                  confirmAlert("Unable to start chat", "Check the backend connection and try again.");
                }
              }}
              disabled={isCreatingSession}
            >
              <Plus size={14} color={theme.colors.text.secondary} />
            </Pressable>
          </View>

          {/* Session cards */}
          <View className="bg-card border border-border rounded-2xl overflow-hidden">
            {section.data.map((session, index) => {
              const status = getStatusStyle(session.status);
              const projectName = getProjectNameForSession(session);
              const branch = getBranchForSession(session);
              const isLast = index === section.data.length - 1;
              const draft = chatSessions[session.id];
              const isDraft = draft ? isDraftSession(draft) : false;
              const preview = isDraft && draft ? draftPreview(draft) : null;

              return (
                <Pressable
                  key={session.id}
                  className={`flex-row items-center px-4 py-3.5 ${
                    !isLast ? "border-b border-border/40" : ""
                  }`}
                  style={({ pressed }) => ({ opacity: pressed ? 0.65 : 1 })}
                  onPressIn={() => prefetchSession?.(session.id)}
                  onPress={() => openSession(session.id)}
                  onLongPress={() => onLongPressSession(session)}
                  delayLongPress={200}
                >
                  {/* Title + project/branch */}
                  <View className="flex-1 mr-2">
                    <View className="flex-row items-center gap-1.5">
                      <Text
                        className="text-sm font-semibold text-foreground mb-0.5 flex-shrink"
                        numberOfLines={1}
                      >
                        {session.title || "Untitled Session"}
                      </Text>
                      {isDraft ? (
                        <View className="px-1.5 py-0.5 rounded bg-amber-500/20 border border-amber-500/30">
                          <Text className="text-[8px] font-bold tracking-widest text-amber-400">
                            DRAFT
                          </Text>
                        </View>
                      ) : null}
                    </View>
                    {isDraft && preview ? (
                      <View className="flex-row items-center gap-1 mb-0.5">
                        {draft && draft.input.trim().length === 0 && draft.attachments.length > 0 ? (
                          <ImageIcon size={10} color="#fcd34d" />
                        ) : null}
                        <Text className="text-xs text-amber-300/90" numberOfLines={1}>
                          {preview}
                        </Text>
                      </View>
                    ) : null}
                    <View className="flex-row items-center gap-1">
                      <Text className="text-xs text-foreground-secondary">{projectName}</Text>
                      {branch ? (
                        <>
                          <Text className="text-xs text-foreground-secondary">•</Text>
                          <Text className="text-xs text-foreground-secondary">{branch}</Text>
                        </>
                      ) : null}
                    </View>
                  </View>

                  {/* Status badge + time */}
                  <View className="items-end gap-1.5">
                    <View
                      className="px-2 py-0.5 rounded-full"
                      style={{ backgroundColor: status.bgColor }}
                    >
                      <Text
                        className="text-[9px] font-bold tracking-wide"
                        style={{ color: status.color }}
                      >
                        {status.label}
                      </Text>
                    </View>
                    <Text className="text-[10px] text-foreground-secondary">
                      {shortRelativeTime(session.updatedAt ?? session.createdAt)}
                    </Text>
                  </View>
                </Pressable>
              );
            })}
          </View>
        </View>
      ))}
    </>
  );
}
