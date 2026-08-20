import { diffLines, type Change } from "diff";

export type DiffLineType = "added" | "removed" | "context";

export interface DiffLine {
  type: DiffLineType;
  text: string;
  oldLineNo?: number;
  newLineNo?: number;
}

export interface DiffResult {
  lines: DiffLine[];
  addedCount: number;
  removedCount: number;
}

/**
 * Splits a chunk of text into individual lines while preserving empty lines.
 */
function splitLines(text: string): string[] {
  const lines = text.split("\n");
  // If the last character is a newline, split creates a trailing empty string. Remove it if the original ended with newline.
  if (lines.length > 0 && lines[lines.length - 1] === "") {
    lines.pop();
  }
  return lines;
}

/**
 * Computes a line-by-line diff between two strings with line numbers.
 */
export function computeLineDiff(oldContent = "", newContent = ""): DiffResult {
  const changes: Change[] = diffLines(oldContent, newContent);
  const lines: DiffLine[] = [];
  let addedCount = 0;
  let removedCount = 0;

  let oldLine = 1;
  let newLine = 1;

  for (const change of changes) {
    const rawLines = splitLines(change.value);

    if (change.added) {
      for (const text of rawLines) {
        lines.push({
          type: "added",
          text,
          newLineNo: newLine++,
        });
        addedCount++;
      }
    } else if (change.removed) {
      for (const text of rawLines) {
        lines.push({
          type: "removed",
          text,
          oldLineNo: oldLine++,
        });
        removedCount++;
      }
    } else {
      for (const text of rawLines) {
        lines.push({
          type: "context",
          text,
          oldLineNo: oldLine++,
          newLineNo: newLine++,
        });
      }
    }
  }

  return {
    lines,
    addedCount,
    removedCount,
  };
}

/**
 * Computes a diff representation for a newly written file where all lines are added.
 */
export function computeNewFileDiff(content = ""): DiffResult {
  const rawLines = splitLines(content);
  const lines: DiffLine[] = rawLines.map((text, idx) => ({
    type: "added",
    text,
    newLineNo: idx + 1,
  }));

  return {
    lines,
    addedCount: lines.length,
    removedCount: 0,
  };
}
