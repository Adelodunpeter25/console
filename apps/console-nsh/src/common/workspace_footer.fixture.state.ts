// Shared state for the workspace footer fixture.
import { signal } from "nsh";
import type { GitBranchInfo, ProjectInfo } from "./workspace_footer";

export const projects = signal<ProjectInfo[]>([]);
export const selectedProjectId = signal<string | null>(null);
export const branches = signal<GitBranchInfo[]>([]);
export const branchLoaded = signal(false);
export const isGitRepository = signal(false);
export const branchPending = signal(false);
export const projectLocked = signal(false);
export const repoCloning = signal<string | null>(null);

export const selectedWith = signal("");
export const lastAction = signal("");
