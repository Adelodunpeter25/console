// The bottom strip under the composer, ported from console-ui's
// `common/workspace_footer.rs`: the project and branch chips, plus the usage
// meter. The project chip lists the backend's known projects plus the
// management rows and the GitHub section; the branch chip lists the selected
// project's Git branches and marks the checked-out one. The parent owns all
// backend calls — this component only paints state and forwards callbacks.

import { Text, View } from "nsh";
import { Menu } from "nsh/base";
import { MenuChip } from "./menu_chip";
import {
  branchMenuSection,
  githubMenuSection,
  projectMenuSection,
  type ProjectMenuExtras,
} from "./picker_menus";
import { UsageMeter, type ContextSnapshot, type UsageReport } from "./usage_meter";

/// console-core's `ProjectInfo` wire shape (proto/console/v1/project.proto).
export interface ProjectInfo {
  id: string;
  name: string;
  path: string;
}

/// console-core's `GitBranchInfo` (proto/console/v1/git.proto).
export interface GitBranchInfo {
  name: string;
  current: boolean;
}

/// console-core's `GithubRepo`: one repository in the connected GitHub
/// account, serialized as plain camelCase JSON by the server.
export interface GithubRepo {
  fullName: string;
  name: string;
  private: boolean;
  description: string;
  htmlUrl: string;
  /// HTTPS clone endpoint — what `clone_project` takes as its url.
  cloneUrl: string;
  pushedAt: string;
}

export interface WorkspaceFooterProps {
  projects: ProjectInfo[] | (() => ProjectInfo[]);
  selectedProjectId: string | null | (() => string | null);
  branches: GitBranchInfo[] | (() => GitBranchInfo[]);
  branchLoaded: boolean | (() => boolean);
  isGitRepository: boolean | (() => boolean);
  branchPending: boolean | (() => boolean);
  /// The session already has messages so the working directory can no longer
  /// change: the project chip shows a lock and does not open.
  projectLocked?: boolean | (() => boolean);
  githubRepos?: GithubRepo[] | null | (() => GithubRepo[] | null);
  githubReposLoading?: boolean | (() => boolean);
  githubReposError?: string | null | (() => string | null);
  /// The repo a GitHub tap is cloning right now, shown on the project chip
  /// ("Cloning <name>…") because the menu itself closes on tap — without
  /// this, a long clone looks like nothing happened.
  repoCloning?: string | null | (() => string | null);
  usageReport: UsageReport | null | (() => UsageReport | null);
  contextSnapshot: ContextSnapshot | null | (() => ContextSnapshot | null);
  usageLoading?: boolean | (() => boolean);
  onSelectProject: (id: string) => void;
  onNewProject: () => void;
  onNoProject: () => void;
  onOpenGithubRepo: (repo: GithubRepo) => void;
  onRefreshGithubRepos: () => void;
  onConnectGithub: () => void;
  onSelectBranch: (name: string) => void;
  onNewWorktree: () => void;
}

function lift<T>(value: T | (() => T)): () => T {
  return typeof value === "function" ? (value as () => T) : () => value;
}

export function WorkspaceFooter(props: WorkspaceFooterProps) {
  const projects = lift(props.projects);
  const selectedProjectId = lift(props.selectedProjectId);
  const branches = lift(props.branches);
  const branchLoaded = lift(props.branchLoaded);
  const isGitRepository = lift(props.isGitRepository);
  const branchPending = lift(props.branchPending);
  const projectLocked = lift(props.projectLocked ?? false);
  const githubRepos = lift(props.githubRepos ?? null);
  const githubReposLoading = lift(props.githubReposLoading ?? false);
  const githubReposError = lift(props.githubReposError ?? null);
  const repoCloning = lift(props.repoCloning ?? null);
  const usageLoading = lift(props.usageLoading ?? false);

  const selectedProject = () =>
    selectedProjectId() == null
      ? null
      : projects().find((project) => project.id === selectedProjectId()) ?? null;

  const projectLabel = () => {
    const cloning = repoCloning();
    if (cloning) return `Cloning ${cloning}…`;
    return selectedProject()?.name ?? "No project";
  };
  const currentBranch = () =>
    branches().find((branch) => branch.current)?.name ?? null;

  const projectExtras = (): ProjectMenuExtras => ({
    onNewProject: props.onNewProject,
    onNoProject: props.onNoProject,
    noneSelected: selectedProjectId() == null,
  });

  // The branch chip is a plain label until there is a project to read a
  // repository from — opening a picker with nothing to pick is noise.
  const branchEnabled = () =>
    selectedProject() != null &&
    branchLoaded() &&
    isGitRepository() &&
    !branchPending() &&
    branches().length > 0;

  const branchLabel = () => {
    if (branchPending()) return "Switching…";
    if (selectedProject() == null) return "No branch";
    if (!branchLoaded()) return "Loading…";
    if (!isGitRepository()) return "No Git repository";
    return currentBranch() ?? "No branch";
  };

  return (
    <View class="w-full bg-card px-5 pt-1 pb-2">
      <View class="mx-auto flex h-7 w-full max-w-[768px] flex-row items-center justify-between gap-0.5 px-2.5 text-[11px] leading-[14px]">
        {projectLocked() ? (
          <MenuChip icon="Lock" label={projectLabel} disabled maxWidth={190} accessibilityLabel="Project" />
        ) : (
          <Menu
            placement="top"
            align="start"
            popupClass="rounded-xl"
            trigger={
              <MenuChip
                icon={projectLocked() ? "Lock" : "Folder"}
                label={projectLabel}
                maxWidth={190}
                accessibilityLabel="Project"
              />
            }
          >
            <View>
              {projectMenuSection({
                projects: projects(),
                selectedId: selectedProjectId(),
                withIcons: false,
                onSelect: props.onSelectProject,
                extras: projectExtras(),
              })}
              {githubMenuSection({
                repos: githubRepos(),
                loading: githubReposLoading(),
                hasError: githubReposError() != null,
                onOpen: props.onOpenGithubRepo,
                onRefresh: props.onRefreshGithubRepos,
                onConnect: props.onConnectGithub,
              })}
            </View>
          </Menu>
        )}

        {branchEnabled() ? (
          <Menu
            placement="top"
            align="start"
            popupClass="rounded-xl"
            trigger={
              <MenuChip icon="GitBranch" label={branchLabel} maxWidth={200} accessibilityLabel="Branch" />
            }
          >
            <View>
              {branchMenuSection({
                branches: branches(),
                isSelected: (branch) => branch.current,
                onSelect: props.onSelectBranch,
                withIcons: false,
                // New worktree spawns a session rooted in a fresh worktree
                // (server auto-names the branch) and opens it — it never
                // re-roots the current session.
                newWorktree: { label: "New worktree…", onNewWorktree: props.onNewWorktree },
              })}
            </View>
          </Menu>
        ) : (
          <MenuChip icon="GitBranch" label={branchLabel} disabled maxWidth={200} accessibilityLabel="Branch" />
        )}

        <UsageMeter
          usageReport={props.usageReport}
          contextSnapshot={props.contextSnapshot}
          isLoading={() => usageLoading()}
        />
      </View>
    </View>
  );
}
