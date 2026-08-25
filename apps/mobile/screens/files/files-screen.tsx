import React, { useState, useCallback, useMemo } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView, BackHandler } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Search, RefreshCw, File as FileIcon } from "lucide-react-native";
import { TextInput } from "react-native";
import { useAppStore, useProjectStore } from "@/stores";
import { useDirectoryChildren, useReadFile, useSearchFiles } from "@/hooks/queries";
import { useDebouncedValue } from "@/hooks/useDebouncedValue";
import { getLanguageFromPath, renderHighlightedLine } from "@/components/common/syntax-highlighter";
import { ScreenHeader } from "@/components/layout/screen-header";
import { FileTreeBrowser } from "@/components/files/FileTreeBrowser";
import { getFilePreviewBlock } from "@console/types";
import { theme } from "@/styles/theme";

/** Selected file for preview; `size` comes from the tree entry stat when known. */
interface SelectedFile {
  readonly path: string;
  readonly size?: number;
}

export function FilesScreen() {
  const insets = useSafeAreaInsets();
  const selectedProjectId = useAppStore((s) => s.selectedProjectId);
  const projects = useProjectStore((s) => s.projects);
  const setActiveTab = useAppStore((s) => s.setActiveTab);
  // Back returns to wherever the user came from (never Files itself),
  // mirroring the terminal screen's behavior.
  const previousTab = useAppStore((s) => s.previousTab);
  const goBack = useCallback(() => {
    setActiveTab(previousTab && previousTab !== "files" ? previousTab : "home");
  }, [previousTab, setActiveTab]);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedFile, setSelectedFile] = useState<SelectedFile | null>(null);
  const selectedFilePath = selectedFile?.path ?? null;

  const project = useMemo(
    () => projects.find((p) => p.id === selectedProjectId) ?? projects[0] ?? null,
    [projects, selectedProjectId],
  );

  const projectRoot = project?.path ?? null;

  // Server-side FFF fuzzy search: keystrokes are debounced and resolved against
  // a warm Rust-native index — the device never loads the whole tree to filter.
  const debouncedQuery = useDebouncedValue(searchQuery.trim(), 350);
  const isSearching = Boolean(projectRoot) && debouncedQuery.length > 0;
  const {
    data: searchResults,
    isFetching: isFetchingSearch,
  } = useSearchFiles(projectRoot, debouncedQuery);

  const {
    data: entries = [],
    isLoading: isPendingEntries,
    isFetching: isFetchingEntries,
    error: entriesError,
    refetch: refetchEntries,
  } = useDirectoryChildren(projectRoot);

  // Gate before fetching: lockfiles, binaries, and oversized files are blocked
  // client-side (stopgap — the canonical gate belongs on the server, see
  // docs/notes/file-preview-gating.md).
  const previewBlock = useMemo(() => {
    if (!selectedFile) return null;
    const fileName = selectedFile.path.split("/").pop() ?? selectedFile.path;
    return getFilePreviewBlock(fileName, selectedFile.size);
  }, [selectedFile]);

  const {
    data: fileData,
    isLoading: isLoadingFile,
    error: fileError,
  } = useReadFile(selectedFilePath ?? "", { enabled: !previewBlock });

  const handleSelectFile = useCallback((absolutePath: string, fileSize?: number) => {
    setSelectedFile({ path: absolutePath, size: fileSize });
  }, []);

  const handleBackFromFile = useCallback(() => {
    setSelectedFile(null);
  }, []);

  // Android back handling: if viewing file, close it; else return to previous tab
  React.useEffect(() => {
    const onBackPress = () => {
      if (selectedFilePath) {
        setSelectedFile(null);
        return true;
      }
      goBack();
      return true;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [selectedFilePath, goBack]);

  const isPending = isPendingEntries || isFetchingEntries;
  const errorMsg = entriesError ? (entriesError as Error).message : null;

  const headerTitle = selectedFilePath
    ? selectedFilePath.split("/").pop() ?? "File"
    : project
      ? project.name
      : "Files";

  const headerSubtitle = selectedFilePath
    ? selectedFilePath.replace(projectRoot ?? "", "").replace(/^\//, "") || selectedFilePath
    : project?.path ?? "No project selected";

  if (!project) {
    return (
      <View className="flex-1 bg-screen">
        <ScreenHeader title="Files" onBack={goBack} />
        <View className="flex-1 items-center justify-center px-6">
          <FileIcon size={28} color={theme.colors.text.muted} />
          <Text className="mt-3 text-sm font-bold text-foreground">No project selected</Text>
          <Text className="mt-1 text-xs text-foreground-muted text-center leading-normal">
            Add a project from Home to browse its files.
          </Text>
          <Pressable
            onPress={() => setActiveTab("home")}
            className="mt-4 px-4 py-2.5 rounded-full bg-foreground"
            style={({ pressed }) => ({ opacity: pressed ? 0.8 : 1 })}
          >
            <Text className="text-xs font-bold text-black">Go to Home</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  // File preview mode
  if (selectedFilePath) {
    return (
      <View className="flex-1 bg-screen">
        <ScreenHeader
          title={headerTitle}
          subtitle={headerSubtitle}
          onBack={handleBackFromFile}
        />

        {previewBlock ? (
          <View className="flex-1 items-center justify-center px-6">
            <FileIcon size={28} color={theme.colors.text.muted} />
            <Text className="mt-3 text-sm font-bold text-foreground">{previewBlock.title}</Text>
            <Text className="mt-1 text-xs text-foreground-muted text-center leading-normal">
              {previewBlock.message}
            </Text>
          </View>
        ) : isLoadingFile ? (
          <View className="flex-1 items-center justify-center gap-3">
            <ActivityIndicator color={theme.colors.text.muted} />
            <Text className="text-xs text-foreground-muted">Loading file…</Text>
          </View>
        ) : fileError ? (
          <View className="flex-1 items-center justify-center px-6">
            <Text className="text-sm font-bold text-foreground">Failed to load file</Text>
            <Text className="mt-1 text-xs text-foreground-muted text-center">
              {(fileError as Error).message}
            </Text>
          </View>
        ) : (
          <ScrollView
            className="flex-1 bg-screen"
            contentContainerStyle={{ padding: 16, paddingBottom: insets.bottom + 16 }}
            showsVerticalScrollIndicator={false}
          >
            {(fileData?.content ?? "").split("\n").map((line, i) => (
              <View key={i} className="flex-row items-stretch">
                {/* Line-number gutter (matches diff-view styling) */}
                <Text
                  className="w-8 shrink-0 text-[10px] leading-4 font-mono text-right pr-2 text-foreground-secondary/40"
                  selectable={false}
                >
                  {i + 1}
                </Text>
                <Text
                  className="flex-1 text-[11px] leading-4 font-mono"
                  style={{ color: theme.colors.text.primary }}
                  selectable
                >
                  {renderHighlightedLine(line, getLanguageFromPath(selectedFilePath ?? undefined), String(i)) || " "}
                </Text>
              </View>
            ))}
          </ScrollView>
        )}
      </View>
    );
  }

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader
        title="Files"
        subtitle={project.name}
        onBack={goBack}
        rightAction={
          <Pressable
            onPress={() => refetchEntries()}
            className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
            style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
          >
            <RefreshCw size={16} color={theme.colors.text.secondary} />
          </Pressable>
        }
      />

      {/* Search bar */}
      <View className="flex-row items-center gap-2 px-3 py-2 border-b border-border bg-card/40">
        <View className="flex-1 flex-row items-center gap-2 px-3 py-2 rounded-xl bg-card border border-border/60">
          <Search size={15} color={theme.colors.text.muted} />
          <TextInput
            placeholder="Search files"
            placeholderTextColor={theme.colors.text.muted}
            value={searchQuery}
            onChangeText={setSearchQuery}
            autoCapitalize="none"
            autoCorrect={false}
            clearButtonMode="while-editing"
            className="flex-1 py-0 text-sm"
            style={{ color: theme.colors.text.primary, padding: 0 }}
          />
        </View>
        {searchQuery.length > 0 ? (
          <Pressable
            onPress={() => setSearchQuery("")}
            className="px-3 py-2 rounded-xl bg-card border border-border"
          >
            <Text className="text-xs font-medium text-foreground-secondary">Clear</Text>
          </Pressable>
        ) : null}
      </View>

      <View className="flex-1">
        <FileTreeBrowser
          entries={entries}
          projectRoot={projectRoot!}
          error={errorMsg}
          isPending={isPending}
          searchQuery={searchQuery}
          searchResults={
            isSearching
              ? (searchResults ?? []).map((r) => ({
                  name: r.absolutePath.split("/").pop() ?? r.relativePath,
                  path: r.absolutePath,
                  isDir: r.isDir,
                }))
              : null
          }
          isSearching={isFetchingSearch && isSearching}
          selectedPath={selectedFilePath}
          onRefresh={() => refetchEntries()}
          onSelectFile={handleSelectFile}
        />
      </View>
    </View>
  );
}

export default FilesScreen;
