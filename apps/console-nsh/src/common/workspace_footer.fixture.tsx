// The workspace footer test fixture: `WorkspaceFooter` over local signals.
// The test cannot reach the fixture's module graph, so the fixture renders
// scenario buttons that move its own state; the callbacks' effects render on
// screen for the tests to observe.
import { WorkspaceFooter } from "./workspace_footer";
import type { GitBranchInfo, ProjectInfo } from "./workspace_footer";
import { Text, View, mount, signal } from "nsh";

const projects = signal<ProjectInfo[]>([]);
const selectedProjectId = signal<string | null>(null);
const branches = signal<GitBranchInfo[]>([]);
const branchLoaded = signal(false);
const isGitRepository = signal(false);
const branchPending = signal(false);
const projectLocked = signal(false);
const repoCloning = signal<string | null>(null);

const selectedWith = signal("");
const lastAction = signal("");

const TWO_PROJECTS: ProjectInfo[] = [
  { id: "p1", name: "Repo One", path: "/repo" },
  { id: "p2", name: "Repo Two", path: "/repo2" },
];

function ScenarioButton(props: { label: string; onPress: () => void }) {
  return (
    <View
      class="rounded border border-border px-1 hover:bg-muted"
      role="button"
      accessibilityLabel={props.label}
      onPress={props.onPress}
    >
      <Text class="text-[10px] text-foreground">{props.label}</Text>
    </View>
  );
}

function Fixture() {
  return (
    <View class="w-full">
      <WorkspaceFooter
        projects={() => projects()}
        selectedProjectId={() => selectedProjectId()}
        branches={() => branches()}
        branchLoaded={() => branchLoaded()}
        isGitRepository={() => isGitRepository()}
        branchPending={() => branchPending()}
        projectLocked={() => projectLocked()}
        repoCloning={() => repoCloning()}
        usageReport={null}
        contextSnapshot={null}
        onSelectProject={(id) => selectedWith.set(id)}
        onNewProject={() => lastAction.set("new-project")}
        onNoProject={() => lastAction.set("no-project")}
        onOpenGithubRepo={(repo) => lastAction.set(`open:${repo.name}`)}
        onRefreshGithubRepos={() => lastAction.set("refresh-repos")}
        onConnectGithub={() => lastAction.set("connect")}
        onSelectBranch={(name) => selectedWith.set(name)}
        onNewWorktree={() => lastAction.set("new-worktree")}
      />
      <View class="flex-row flex-wrap gap-1 p-1">
        <ScenarioButton
          label="Load projects"
          onPress={() => {
            projects.set(TWO_PROJECTS);
            selectedProjectId.set(null);
          }}
        />
        <ScenarioButton
          label="Select p2"
          onPress={() => selectedProjectId.set("p2")}
        />
        <ScenarioButton label="Lock" onPress={() => projectLocked.set(true)} />
        <ScenarioButton
          label="Cloning"
          onPress={() => repoCloning.set("acme/api")}
        />
        <ScenarioButton
          label="No git"
          onPress={() => {
            selectedProjectId.set("p1");
            projects.set(TWO_PROJECTS);
            isGitRepository.set(false);
            branchLoaded.set(true);
            branchPending.set(false);
          }}
        />
        <ScenarioButton
          label="Branch loading"
          onPress={() => {
            isGitRepository.set(true);
            branchLoaded.set(false);
            branchPending.set(false);
          }}
        />
        <ScenarioButton
          label="Branch switching"
          onPress={() => {
            isGitRepository.set(true);
            branchLoaded.set(true);
            branchPending.set(true);
          }}
        />
        <ScenarioButton
          label="Branches ready"
          onPress={() => {
            isGitRepository.set(true);
            branchLoaded.set(true);
            branchPending.set(false);
            branches.set([
              { name: "main", current: true },
              { name: "feature/ui", current: false },
            ]);
          }}
        />
      </View>
      <Text>Selected: {selectedWith() || "none"}</Text>
      <Text>Action: {lastAction() || "none"}</Text>
    </View>
  );
}

mount(Fixture);
