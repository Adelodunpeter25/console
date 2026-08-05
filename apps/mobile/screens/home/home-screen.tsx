import React, { useMemo, useState } from "react";
import { Text, View, TouchableOpacity, ScrollView, StyleSheet } from "react-native";
import { GitBranch } from "lucide-react-native";
import { useProjects, useSessions } from "@console/api";
import { SessionHeader } from "@console/types";
import { ScreenHeader } from "../../components/common/screen-header";
import { SearchBar } from "../../components/common/search-bar";
import { useAppStore } from "../../stores";
import { formatRelativeTime } from "../../utils/time";
import { theme } from "../../styles/theme";

interface GroupedSection {
  projectId: string | null;
  projectName: string;
  data: SessionHeader[];
}

function sessionStatus(status?: string): {
  label: string;
  textColor: string;
  bgColor: string;
  rawColor: string;
  rawBgColor: string;
} {
  switch (status) {
    case "working":
      return {
        label: "Running",
        textColor: "text-orange-400",
        bgColor: "bg-orange-400/10",
        rawColor: theme.colors.status.running,
        rawBgColor: theme.colors.status.runningBg,
      };
    case "done":
      return {
        label: "Ready",
        textColor: "text-emerald-400",
        bgColor: "bg-emerald-400/10",
        rawColor: theme.colors.status.ready,
        rawBgColor: theme.colors.status.readyBg,
      };
    case "needs_attention":
      return {
        label: "Needs Attention",
        textColor: "text-red-400",
        bgColor: "bg-red-400/10",
        rawColor: theme.colors.status.attention,
        rawBgColor: theme.colors.status.attentionBg,
      };
    default:
      return {
        label: "Idle",
        textColor: "text-zinc-400",
        bgColor: "bg-zinc-500/10",
        rawColor: theme.colors.status.idle,
        rawBgColor: theme.colors.status.idleBg,
      };
  }
}

function shortRelativeTime(dateInput?: string | number): string {
  const full = formatRelativeTime(dateInput);
  return full.replace(" ago", "").replace("just now", "now");
}

export function HomeScreen() {
  const { data: projects = [] } = useProjects();
  const { data: sessions = [] } = useSessions();
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const [searchQuery, setSearchQuery] = useState("");

  const filteredSessions = useMemo(() => {
    if (!searchQuery.trim()) return sessions;
    const query = searchQuery.toLowerCase();
    return sessions.filter((s) => s.title?.toLowerCase().includes(query));
  }, [sessions, searchQuery]);

  const sections = useMemo<GroupedSection[]>(() => {
    const byProject = new Map<string | null, SessionHeader[]>();
    for (const session of filteredSessions) {
      const list = byProject.get(session.projectId ?? null) ?? [];
      list.push(session);
      byProject.set(session.projectId ?? null, list);
    }

    const result: GroupedSection[] = [];
    for (const [projectId, list] of byProject) {
      const project = projects.find((p) => p.id === projectId);
      const projectName = project ? project.name.toUpperCase() : "DRAFT";
      result.push({
        projectId,
        projectName,
        data: list.sort((a, b) => (b.updatedAt || b.createdAt) - (a.updatedAt || a.createdAt)),
      });
    }
    return result.sort((a, b) => a.projectName.localeCompare(b.projectName));
  }, [filteredSessions, projects]);

  const openSession = (sessionId: string) => {
    setSelectedSessionId(sessionId);
    setActiveTab("chat");
  };

  return (
    <View style={styles.container}>
      <ScreenHeader
        title="T3 Code"
        subtitle="Alpha"
        showSettings
        onSettingsPress={() => setActiveTab("settings")}
        showFilter
      />

      <ScrollView style={styles.scrollArea} contentContainerStyle={styles.scrollContent}>
        {sections.length === 0 ? (
          <View style={styles.emptyContainer}>
            <Text style={styles.emptyText}>No sessions found</Text>
          </View>
        ) : (
          sections.map((section) => (
            <View key={section.projectId ?? "draft"} style={styles.sectionWrapper}>
              <View style={styles.sectionHeader}>
                <Text style={styles.sectionTitle}>📁 {section.projectName}</Text>
                <Text style={styles.sectionCount}>{section.data.length} more</Text>
              </View>

              <View style={styles.cardContainer}>
                {section.data.map((session, index) => {
                  const project = projects.find((p) => p.id === session.projectId);
                  const status = sessionStatus(session.status);
                  const projectName = project ? project.name : "DeskMini";
                  const isLast = index === section.data.length - 1;

                  return (
                    <TouchableOpacity
                      key={session.id}
                      style={[styles.sessionRow, isLast && styles.lastRow]}
                      onPress={() => openSession(session.id)}
                      activeOpacity={0.7}
                    >
                      <View style={[styles.avatarBox, { backgroundColor: status.rawBgColor }]}>
                        <GitBranch size={16} color={status.rawColor} />
                      </View>

                      <View style={styles.middleContent}>
                        <Text style={styles.sessionTitleText} numberOfLines={1}>
                          {session.title || "Untitled Session"}
                        </Text>
                        <View style={styles.detailRow}>
                          <Text style={styles.detailText}>{projectName}</Text>
                          <Text style={styles.divider}>•</Text>
                          <Text style={styles.detailText}>main</Text>
                        </View>
                      </View>

                      <View style={styles.rightContent}>
                        <View style={[styles.statusBadge, { backgroundColor: status.rawBgColor }]}>
                          <Text style={[styles.statusBadgeText, { color: status.rawColor }]}>
                            {status.label}
                          </Text>
                        </View>
                        <Text style={styles.timeText}>
                          {shortRelativeTime(session.updatedAt || session.createdAt)}
                        </Text>
                      </View>
                    </TouchableOpacity>
                  );
                })}
              </View>
            </View>
          ))
        )}
      </ScrollView>

      {/* Floating Search Bar Component */}
      <SearchBar
        value={searchQuery}
        onChangeText={setSearchQuery}
        onComposePress={() => {
          // Action to create new session
          setActiveTab("chat");
          setSelectedSessionId(null);
        }}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  scrollArea: {
    flex: 1,
  },
  scrollContent: {
    paddingHorizontal: 16,
    paddingTop: 8,
    paddingBottom: 40,
  },
  emptyContainer: {
    alignItems: "center",
    justifyContent: "center",
    paddingVertical: 60,
  },
  emptyText: {
    color: theme.colors.text.muted,
    fontSize: 14,
  },
  sectionWrapper: {
    marginBottom: 20,
  },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 8,
    paddingHorizontal: 4,
  },
  sectionTitle: {
    fontSize: 12,
    fontWeight: "bold",
    color: theme.colors.text.muted,
    letterSpacing: 1,
    textTransform: "uppercase",
  },
  sectionCount: {
    fontSize: 11,
    color: theme.colors.text.muted,
    opacity: 0.7,
  },
  cardContainer: {
    backgroundColor: theme.colors.surface,
    borderColor: theme.colors.border,
    borderWidth: 1,
    borderRadius: theme.roundness.lg,
    overflow: "hidden",
  },
  sessionRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 14,
    borderBottomWidth: 1,
    borderBottomColor: theme.colors.borderSubtle,
  },
  lastRow: {
    borderBottomWidth: 0,
  },
  avatarBox: {
    width: 36,
    height: 36,
    borderRadius: theme.roundness.md,
    alignItems: "center",
    justifyContent: "center",
    marginRight: 14,
  },
  middleContent: {
    flex: 1,
    marginRight: 10,
  },
  sessionTitleText: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.colors.text.primary,
    marginBottom: 4,
  },
  detailRow: {
    flexDirection: "row",
    alignItems: "center",
  },
  detailText: {
    fontSize: 11,
    color: theme.colors.text.muted,
  },
  divider: {
    fontSize: 11,
    color: theme.colors.text.muted,
    marginHorizontal: 5,
  },
  rightContent: {
    alignItems: "end",
  },
  statusBadge: {
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: theme.roundness.full,
    marginBottom: 6,
  },
  statusBadgeText: {
    fontSize: 9,
    fontWeight: "bold",
  },
  timeText: {
    fontSize: 10,
    color: theme.colors.text.muted,
  },
});

export default HomeScreen;
