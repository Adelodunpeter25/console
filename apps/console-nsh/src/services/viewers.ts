// Viewers' data loaders: file contents, git diffs, and session file
// changes (the review tab's list). The workspace tab components read only
// through here, keeping the bridge seam in `backend.ts`.

import { backend } from "./backend";

export type FsFileContent = { path: string; content: string };
export type GitDiffResponse = { path: string; diff: string };

/// One file a session's run changed (the server's SessionFileChange row).
export type SessionFileChange = {
  path: string;
  turnIndex: number;
  status: string;
  additions: number;
  deletions: number;
  diffText: string | null;
  reviewed: boolean;
  updatedAt: string;
  userMessageId: string;
};

export const viewers = {
  fileContent: (baseUrl: string, path: string) =>
    backend.fsReadFile(baseUrl, path).then((s) => JSON.parse(s) as FsFileContent),

  fileDiff: (baseUrl: string, repoPath: string, filePath: string | null) =>
    backend.gitDiff(baseUrl, repoPath, filePath).then((s) => JSON.parse(s) as GitDiffResponse),

  sessionChanges: (baseUrl: string, sessionId: string, turnIndex: number | null) =>
    backend
      .sessionChanges(baseUrl, sessionId, turnIndex)
      .then((s) => JSON.parse(s) as SessionFileChange[]),

  changeDiff: (baseUrl: string, sessionId: string, path: string, turnIndex: number) =>
    backend.sessionChangeDiff(baseUrl, sessionId, path, turnIndex),

  setChangeReviewed: (
    baseUrl: string,
    sessionId: string,
    path: string,
    turnIndex: number,
    reviewed: boolean,
  ) => backend.setChangeReviewed(baseUrl, sessionId, path, turnIndex, reviewed),
};
