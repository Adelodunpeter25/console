/**
 * File-preview gating rules shared by the server (`GET /api/fs/file`) and
 * clients (mobile Files screen pre-fetch fast path).
 *
 * The server is the enforcement layer of record (see
 * docs/notes/file-preview-gating.md); clients use the same predicates to block
 * before a network round trip.
 */

/** Files larger than this are rejected for text preview (~512 KB). */
export const MAX_FILE_PREVIEW_BYTES = 512 * 1024;

/** Structured codes returned by `GET /api/fs/file` when a preview is blocked. */
export type FilePreviewBlockedCode = "LOCKFILE_BLOCKED" | "BINARY_FILE" | "FILE_TOO_LARGE";

export type FilePreviewBlockKind = FilePreviewBlockedCode;

export interface FilePreviewBlock {
  readonly kind: FilePreviewBlockKind;
  /** Short title for a blocked-preview panel. */
  readonly title: string;
  /** Human-readable explanation shown under the title. */
  readonly message: string;
}

/** Exact basenames of common lock/manifest files that end in non-`.lock` names. */
const LOCK_FILE_BASENAMES = new Set([
  "package-lock.json",
  "npm-shrinkwrap.json",
  "pnpm-lock.yaml",
  "composer.lock",
]);

/** Extensions that are machine-generated lockfiles (covers *.lock, bun.lockb, Cargo.lock…). */
const LOCK_FILE_SUFFIXES = [".lock", ".lockb"];

/** Extensions that should never be rendered as text in a preview pane. */
const BINARY_FILE_EXTENSIONS = new Set([
  // Images
  ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".icns", ".tiff",
  // Video / audio
  ".mp4", ".mov", ".avi", ".mkv", ".webm", ".mp3", ".wav", ".flac", ".ogg", ".m4a",
  // Archives
  ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".jar", ".war",
  // Executables / objects
  ".exe", ".dll", ".so", ".dylib", ".a", ".o", ".obj", ".bin", ".iso", ".dmg",
  ".pkg", ".deb", ".rpm", ".apk", ".ipa", ".node",
  // Fonts
  ".ttf", ".otf", ".woff", ".woff2", ".eot",
  // Documents / other opaque formats
  ".pdf", ".wasm", ".psd", ".sketch", ".class", ".pyc", ".db", ".sqlite", ".sqlite3",
]);

function extensionOf(fileName: string): string {
  const idx = fileName.lastIndexOf(".");
  return idx === -1 ? "" : fileName.slice(idx).toLowerCase();
}

export function isLockFileName(fileName: string): boolean {
  const lower = fileName.toLowerCase();
  if (LOCK_FILE_BASENAMES.has(lower)) return true;
  return LOCK_FILE_SUFFIXES.some((suffix) => lower.endsWith(suffix));
}

export function isBinaryFileName(fileName: string): boolean {
  return BINARY_FILE_EXTENSIONS.has(extensionOf(fileName));
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * Decide whether a file may be opened as a text preview.
 * Returns `null` when the file is allowed, otherwise a block reason.
 *
 * @param fileName basename of the file (e.g. "bun.lockb")
 * @param sizeBytes optional size from stat/tree entry; when unknown,
 * name-based rules still apply and only the size cap is skipped.
 */
export function getFilePreviewBlock(
  fileName: string,
  sizeBytes?: number,
): FilePreviewBlock | null {
  if (isLockFileName(fileName)) {
    return {
      kind: "LOCKFILE_BLOCKED",
      title: "Lockfiles can't be previewed",
      message: `"${fileName}" is a generated lockfile — open it on your machine instead.`,
    };
  }
  if (isBinaryFileName(fileName)) {
    return {
      kind: "BINARY_FILE",
      title: "Binary file",
      message: `"${fileName}" isn't a text file, so there's nothing to preview here.`,
    };
  }
  if (sizeBytes !== undefined && sizeBytes > MAX_FILE_PREVIEW_BYTES) {
    return {
      kind: "FILE_TOO_LARGE",
      title: "File too large",
      message: `"${fileName}" is ${formatBytes(sizeBytes)} — previews are capped at ${formatBytes(MAX_FILE_PREVIEW_BYTES)}.`,
    };
  }
  return null;
}
