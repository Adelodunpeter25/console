// The project, branch and GitHub menu builders shared by the two pickers —
// the docked footer's dropdowns and the floating composer card's later.
// Ported from console-ui's `common/picker_menus.rs` and `github_menu.rs`.
// Like there, the parent owns all backend calls and callbacks; these only
// paint rows and forward taps, so the two surfaces cannot drift apart.

import { Text, View } from "nsh";
import type { JSX } from "nsh";
import { MenuItem, MenuSeparator } from "nsh/base";
import { Icon } from "./icons";
import type {
  GitBranchInfo,
  GithubRepo,
  ProjectInfo,
} from "./workspace_footer";

/// A small section title row (the GitHub header) — desktop's `MenuItem::Header`.
function MenuHeader(props: { children: string }) {
  return (
    <View class="px-2 pt-1.5 pb-0.5">
      <Text class="text-[10.5px] font-semibold tracking-wide text-muted-foreground">
        {props.children}
      </Text>
    </View>
  );
}

function InertRow(props: { children: string }) {
  return (
    <MenuItem disabled>
      <Text class="text-[12.5px] text-muted-foreground">{props.children}</Text>
    </MenuItem>
  );
}

/// Rows only the docked footer offers: "New project…" and "No project". The
/// floating card creates sessions from existing projects, so it passes none.
export interface ProjectMenuExtras {
  onNewProject: () => void;
  onNoProject: () => void;
  /// Whether "No project" is the current selection.
  noneSelected: boolean;
}

/// The project list, optionally followed by the footer's management rows.
/// `withIcons` is the one cosmetic difference between the surfaces — the
/// card's rows carry folder icons, the footer's keep the plain look.
export function projectMenuSection(props: {
  projects: ProjectInfo[];
  selectedId: string | null;
  withIcons: boolean;
  onSelect: (id: string) => void;
  extras?: ProjectMenuExtras;
}): JSX.Element[] {
  const items: JSX.Element[] = props.projects.map((project) => (
    <MenuItem
      checked={props.selectedId === project.id}
      role="menuitemradio"
      onSelect={() => props.onSelect(project.id)}
    >
      <View class="flex-row items-center gap-1.5">
        {props.withIcons ? <Icon name="Folder" size={12} /> : null}
        <Text class="text-[12.5px] text-foreground">{project.name}</Text>
      </View>
    </MenuItem>
  ));

  const extras = props.extras;
  if (!extras) return items;
  if (items.length > 0) items.push(<MenuSeparator class="h-px bg-border" />);
  items.push(
    <MenuItem onSelect={extras.onNewProject}>
      <View class="flex-row items-center gap-1.5">
        <Icon name="FolderNew" size={12} />
        <Text class="text-[12.5px] text-foreground">New project…</Text>
      </View>
    </MenuItem>,
  );
  items.push(
    <MenuItem

      checked={extras.noneSelected}
      role="menuitemradio"
      onSelect={extras.onNoProject}
    >
      <View class="flex-row items-center gap-1.5">
        <Icon name="X" size={12} />
        <Text class="text-[12.5px] text-foreground">No project</Text>
      </View>
    </MenuItem>,
  );
  return items;
}

/// The branch list. The card prepends its mode rows (not needed by the
/// footer); the footer appends a "New worktree…" action row via
/// `newWorktreeLabel`, and renders "No branches" when the list is empty.
/// `isSelected` says which branch row carries the check.
export function branchMenuSection(props: {
  branches: GitBranchInfo[];
  isSelected: (branch: GitBranchInfo) => boolean;
  onSelect: (name: string) => void;
  withIcons: boolean;
  newWorktree?: { label: string; onNewWorktree: () => void };
}): JSX.Element[] {
  const items: JSX.Element[] = [];

  if (props.branches.length === 0) {
    items.push(<InertRow>No branches</InertRow>);
  }
  for (const branch of props.branches) {
    items.push(
      <MenuItem
        checked={props.isSelected(branch)}
        role="menuitemradio"
        onSelect={() => props.onSelect(branch.name)}
      >
        <View class="flex-row items-center gap-1.5">
          {props.withIcons ? <Icon name="GitBranch" size={12} /> : null}
          <Text class="text-[12.5px] text-foreground">{branch.name}</Text>
        </View>
      </MenuItem>,
    );
  }
  if (props.newWorktree) {
    if (items.length > 0) items.push(<MenuSeparator class="h-px bg-border" />);
    items.push(
      <MenuItem

        onSelect={props.newWorktree.onNewWorktree}
      >
        <View class="flex-row items-center gap-1.5">
          <Icon name="GitBranch" size={12} />
          <Text class="text-[12.5px] text-foreground">
            {props.newWorktree.label}
          </Text>
        </View>
      </MenuItem>,
    );
  }
  return items;
}

/// The GitHub section of the project menu: loading, the connected account's
/// repos (first 20) with a Refresh row, the error+Retry pair, or the connect
/// handoff. The parent owns the fetch; the error text is logged there.
export function githubMenuSection(props: {
  repos: GithubRepo[] | null;
  loading: boolean;
  hasError: boolean;
  onOpen: (repo: GithubRepo) => void;
  onRefresh: () => void;
  onConnect: () => void;
}): JSX.Element[] {
  const items: JSX.Element[] = [
    <MenuHeader>GitHub</MenuHeader>,
  ];
  if (props.loading) {
    items.push(<InertRow>Loading repositories…</InertRow>);
    return items;
  }
  if (props.repos) {
    if (props.repos.length === 0) {
      items.push(<InertRow>No repositories found</InertRow>);
    }
    for (const repo of props.repos.slice(0, 20)) {
      items.push(
        <MenuItem
          onSelect={() => props.onOpen(repo)}
        >
          <View class="flex-row items-center gap-1.5">
            <Icon name="Github" size={12} mono={false} />
            <Text class="text-[12.5px] text-foreground">{repo.name}</Text>
          </View>
        </MenuItem>,
      );
    }
    items.push(
      <MenuItem onSelect={props.onRefresh}>
        <View class="flex-row items-center gap-1.5">
          <Icon name="Refresh" size={12} />
          <Text class="text-[12.5px] text-foreground">Refresh</Text>
        </View>
      </MenuItem>,
    );
    return items;
  }
  if (props.hasError) {
    // A real failure (network, server). The text is too long for a menu row —
    // it is logged by the caller — so the row stays inert and Retry sits under.
    items.push(<InertRow>Couldn't load repositories</InertRow>);
    items.push(
      <MenuItem onSelect={props.onRefresh}>
        <View class="flex-row items-center gap-1.5">
          <Icon name="Refresh" size={12} />
          <Text class="text-[12.5px] text-foreground">Retry</Text>
        </View>
      </MenuItem>,
    );
    return items;
  }
  // Not connected: the fetch reports this as a state, not an error. Offer the
  // handoff to Settings > Providers.
  items.push(
    <MenuItem onSelect={props.onConnect}>
      <View class="flex-row items-center gap-1.5">
        <Icon name="Github" size={12} mono={false} />
        <Text class="text-[12.5px] text-foreground">Connect GitHub…</Text>
      </View>
    </MenuItem>,
  );
  return items;
}
