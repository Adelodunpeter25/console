// The floating compact composer, ported from console-ui's
// `floating_composer/` (the Cmd+N launcher card): a collapsed pill that
// expands into a launcher — project, branch/worktree and GitHub header menus
// over the full `Composer` — and collapses again with Escape over an empty
// draft. The desktop card submits a brand-new session (`BranchChoice` picks
// project folder / new worktree / existing branch); the submit here hands
// `onLaunch` the draft and the picked checkout, and the session creation
// itself lands with the session wiring.
//
// NSH renders components once and tracks prop reads in JSX, so the draft
// emptiness is read inside the keymap handler rather than normalized into a
// body-level const (which would freeze it at mount).

import { Text, View, keymap, signal, token } from "nsh";
import { Popover } from "nsh/base";
import { Icon } from "./icons";
import { MenuChip } from "./menu_chip";
import {
  branchMenuSection,
  githubMenuSection,
  projectMenuSection,
} from "./picker_menus";
import type {
  GitBranchInfo,
  GithubRepo,
  ProjectInfo,
} from "./workspace_footer";
import { Composer } from "./composer";
import {
  ModelDropdownMenu,
  formatModelName,
  type CatalogProvider,
  type PickerTab,
  type SelectedModel,
} from "./model_picker";

/// The checkout a launched session runs in (the desktop's `BranchChoice`).
export type LauncherCheckout =
  | { kind: "project" }
  | { kind: "newWorktree" }
  | { kind: "branch"; name: string };

export function FloatingComposer(props: Parameters<typeof Composer>[0] & {
  projects?: ProjectInfo[];
  branches?: GitBranchInfo[];
  repos?: GithubRepo[] | null;
  /// Called with the draft and the picked checkout when the card submits.
  onLaunch?: (draft: string, checkout: LauncherCheckout) => void;
  /// The model catalog (the launcher's model chip + picker). Absent until
  /// the catalog store loads, matching the desktop's launcher card.
  modelCatalog?: {
    providers: CatalogProvider[];
    favorites: Set<string>;
    selected: SelectedModel | null;
    onSelect: (provider: string, modelId: string) => void;
    onFavorite: (provider: string, modelId: string) => void;
  };
}) {
  const open = signal(false);
  const selectedProjectId = signal<string | null>(null);
  const checkout = signal<LauncherCheckout>({ kind: "project" });
  // The model picker's card-local state (desktop owns it on the app entity;
  // the launcher card is this app's picker host, so card-local signals do).
  const pickerTab = signal<PickerTab>({ kind: "favorites" });
  const pickerSearch = signal("");
  // Escape over an empty draft collapses the card back to the pill; a
  // draft keeps the focus (and the escape for the autocomplete).
  function EscapeCollapses() {
    keymap.bind("escape", () => {
      if (props.value === "") open.set(false);
    });
    return null;
  }

  const projectName = () =>
    props.projects?.find((p) => p.id === selectedProjectId())?.name ??
    "Select project";

  const checkoutLabel = (): string => {
    const choice = checkout();
    if (choice.kind === "newWorktree") return "New worktree";
    if (choice.kind === "branch") return choice.name;
    return "Project folder";
  };

  const modelLabel = (): string => {
    const selected = props.modelCatalog?.selected;
    return selected != null ? formatModelName(selected.modelId) : "Model";
  };

  const launch = () => {
    if (props.value.trim() === "") return;
    props.onLaunch?.(props.value, checkout());
    open.set(false);
  };

  return (
    <View class="w-full">
      {open() ? (
        <View class="flex-col gap-2 rounded-xl border border-border bg-card p-2">
          <EscapeCollapses />
          <View class="flex-row items-center gap-1.5 px-1">
            {props.projects != null ? (
              <Popover
                placement="bottom"
                align="start"
                trigger={
                  <MenuChip
                    icon="Folder"
                    label={projectName}
                    accessibilityLabel="Project menu"
                  />
                }
              >
                <View class="w-[220px] flex-col py-1">
                  {projectMenuSection({
                    projects: props.projects,
                    selectedId: selectedProjectId(),
                    withIcons: false,
                    onSelect: (id) => selectedProjectId.set(id),
                  })}
                </View>
              </Popover>
            ) : null}
            {props.branches != null ? (
              <Popover
                placement="bottom"
                align="start"
                trigger={
                  <MenuChip
                    icon="GitBranch"
                    label={checkoutLabel}
                    accessibilityLabel="Branch menu"
                  />
                }
              >
                <View class="w-[220px] flex-col py-1">
                  {branchMenuSection({
                    branches: props.branches,
                    isSelected: (b) => {
                      const choice = checkout();
                      return choice.kind === "branch" && choice.name === b.name;
                    },
                    onSelect: (name) => checkout.set({ kind: "branch", name }),
                    withIcons: false,
                    newWorktree: {
                      label: "New worktree",
                      onNewWorktree: () => checkout.set({ kind: "newWorktree" }),
                    },
                  })}
                  <View class="px-2 py-1">
                    <View
                      class="flex-row items-center gap-1.5 rounded-md px-2 py-1 hover:bg-accent"
                      role="menuitemradio"
                      accessibilityState={{
                        selected: checkout().kind === "project",
                      }}
                      accessibilityLabel="Project folder"
                      onPress={() => checkout.set({ kind: "project" })}
                    >
                      <Icon
                        name="Folder"
                        size={12}
                        color={token("muted-foreground")}
                      />
                      <Text class="text-[12.5px] text-foreground">
                        Project folder
                      </Text>
                    </View>
                  </View>
                </View>
              </Popover>
            ) : null}
            {props.repos != null ? (
              <Popover
                placement="bottom"
                align="start"
                trigger={
                  <MenuChip
                    icon="Github"
                    label="GitHub"
                    accessibilityLabel="GitHub menu"
                  />
                }
              >
                <View class="w-[240px] flex-col py-1">
                  {githubMenuSection({
                    repos: props.repos,
                    loading: false,
                    hasError: false,
                    onOpen: () => {},
                    onRefresh: () => {},
                    onConnect: () => {},
                  })}
                </View>
              </Popover>
            ) : null}
            {props.modelCatalog != null ? (
              <Popover
                placement="bottom"
                align="start"
                trigger={
                  <MenuChip
                    icon="Sparkle"
                    label={modelLabel}
                    accessibilityLabel="Model menu"
                  />
                }
              >
                <ModelDropdownMenu
                  providers={props.modelCatalog.providers}
                  selected={() => props.modelCatalog?.selected ?? null}
                  activeTab={() => pickerTab()}
                  favorites={() => props.modelCatalog?.favorites ?? new Set()}
                  searchQuery={() => pickerSearch()}
                  onSearch={(query: string) => pickerSearch.set(query)}
                  onSelect={(provider: string, modelId: string) =>
                    props.modelCatalog?.onSelect(provider, modelId)
                  }
                  onTab={(tab: PickerTab) => pickerTab.set(tab)}
                  onFavorite={(provider: string, modelId: string) =>
                    props.modelCatalog?.onFavorite(provider, modelId)
                  }
                />
              </Popover>
            ) : null}
            <View class="flex-1" />
            <View
              class="flex h-6 w-6 items-center justify-center rounded-md hover:bg-accent"
              role="button"
              accessibilityLabel="Close launcher"
              onPress={() => open.set(false)}
            >
              <Icon name="X" size={12} color={token("muted-foreground")} />
            </View>
          </View>
          <Composer {...props} onSend={launch} />
        </View>
      ) : (
        <View
          class="flex-row items-center gap-2 rounded-full border border-border bg-card px-4 py-2.5"
          role="button"
          accessibilityLabel="Ask Console anything"
          onPress={() => open.set(true)}
        >
          <Icon name="Compose" size={14} color={token("muted-foreground")} />
          <Text class="flex-1 text-sm text-muted-foreground">
            Ask Console anything…
          </Text>
          <Icon name="Paperclip" size={14} color={token("muted-foreground")} />
        </View>
      )}
    </View>
  );
}
