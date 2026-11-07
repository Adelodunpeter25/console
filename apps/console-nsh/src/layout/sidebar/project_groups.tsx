// The project-sorted sidebar sections, ported from console-ui's
// `layout/sidebar/mod.rs` (the `SidebarSection::Project` path): one
// collapsible section per project over `session_groups.ts`'s
// `groupSessionsByProject`, each section's chats as `SessionItem` rows. The
// collapsed keys are the parent's — the GPUI app also keeps collapse state
// outside the sidebar.

import { View } from "nsh";
import type { SessionSummary } from "console-nsh/native";
import { GroupHeader } from "./group_header";
import { SessionItem } from "../session/session_item";
import { groupSessionsByProject } from "../session/session_groups";

export function ProjectGroups(props: {
  sessions: SessionSummary[];
  /** Resolves a chat's project to the section's label. */
  projectName: (projectId: string) => string | null;
  activeSessionId: string | null;
  /** The collapsed sections, by their project label (`groupSessionsByProject`'s). */
  collapsed: (label: string) => boolean;
  onToggleProject: (label: string) => void;
  onSession?: (id: string) => void;
  onToggleSettle?: (id: string) => void;
  onDeleteSession?: (id: string) => void;
}) {
  return (
    <View class="flex flex-col gap-1 px-2 pb-3">
      {groupSessionsByProject(props.sessions, props.projectName).map(
        (group) => {
          const collapsed = props.collapsed(group.label);
          return (
            <View class="flex flex-col">
              <GroupHeader
                label={group.label}
                collapsed={collapsed}
                onToggle={() => props.onToggleProject(group.label)}
              />
              {collapsed
                ? null
                : group.sessions.map((s) => (
                    <SessionItem
                      title={s.title}
                      updatedAt={s.updatedAt}
                      active={props.activeSessionId === s.id}
                      projectName={
                        s.projectId ? props.projectName(s.projectId) : null
                      }
                      onPress={() => props.onSession?.(s.id)}
                      onToggleSettle={
                        props.onToggleSettle
                          ? () => props.onToggleSettle?.(s.id)
                          : undefined
                      }
                      onDelete={
                        props.onDeleteSession
                          ? () => props.onDeleteSession?.(s.id)
                          : undefined
                      }
                    />
                  ))}
            </View>
          );
        },
      )}
    </View>
  );
}
