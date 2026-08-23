import React, { useState, useCallback, useMemo } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView, BackHandler } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Search, RefreshCw, File as FileIcon, ArrowLeft } from "lucide-react-native";
import { TextInput } from "react-native";
import { ScreenHeader } from "../../components/layout/screen-header";
import { FileTreeBrowser } from "../../components/files/FileTreeBrowser";
import { useAppStore, useProjectStore } from "../../stores";
import { useFsEntries, useReadFile } from "../../hooks/queries";
import { theme } from "../../styles/theme";

export function FilesScreen() {
  const insets = useSafeAreaInsets();
  const selectedProjectId = useAppStore((s) => s.selectedProjectId);
  const projects = useProjectStore((s) => s.projects);
  const setActiveTab = useAppStore((s) => s.setActiveTab);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null);

  const project = useMemo(
    () => projects.find((p) => p.id === selectedProjectId) ?? projects[0] ?? null,
    [projects, selectedProjectId],
  );

  const projectRoot = project?.path ?? null;

  const {
    data: entries = [],
    isLoading: isPendingEntries,
    isFetching: isFetchingEntries,
    error: entriesError,
    refetch: refetchEntries,
  } = useFsEntries(projectRoot, 6);

  const {
    data: fileData,
    isLoading: isLoadingFile,
    error: fileError,
  } = useReadFile(selectedFilePath ?? "");

  const handleSelectFile = useCallback((absolutePath: string) => {
    setSelectedFilePath(absolutePath);
  }, []);

  const handleBackFromFile = useCallback(() => {
    setSelectedFilePath(null);
  }, []);

  // Android back handling: if viewing file, go back to tree; else go home
  React.useEffect(() => {
    const onBackPress = () => {
      if (selectedFilePath) {
        setSelectedFilePath(null);
        return true;
      }
      return false;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [selectedFilePath]);

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
        <ScreenHeader title="Files" onBack={() => setActiveTab("home")} />
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
          rightAction={
            <Pressable
              onPress={() => handleBackFromFile()}
              className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
            >
              <ArrowLeft size={18} color={theme.colors.text.secondary} />
            </Pressable>
          }
        />

        {isLoadingFile ? (
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
            <Text
              selectable
              className="text-xs leading-5 font-mono"
              style={{ color: theme.colors.text.primary }}
            >
              {fileData?.content ?? ""}
            </Text>
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
        onBack={() => setActiveTab("home")}
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
          selectedPath={selectedFilePath}
          onRefresh={() => refetchEntries()}
          onSelectFile={handleSelectFile}
        />
      </View>
    </View>
  );
}

export default FilesScreen;
