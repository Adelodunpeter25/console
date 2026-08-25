/**
 * Client-side guards for the file preview (Files screen).
 *
 * Blocks opening files that are useless or harmful to render on a phone:
 * generated lockfiles, binary blobs, and anything over the size cap.
 *
 * NOTE: This is a stopgap enforced on mobile only. The canonical gate belongs
 * on the server (`GET /api/fs/file`) so every client is protected — see
 * docs/notes/file-preview-gating.md.
 */

/** Files larger than this are rejected before fetching (~512 KB). */
export const MAX_FILE_PREVIEW_BYTES = 512 * 1024;

/** Exact basenames of common lock/manifest files that end in non-`.lock` names. */
const LOCK_FILE_BASENAMES = new Set([
  "package-lock.json",
  "npm-shrinkwrap.json",
  "pnpm-lock.yaml",
  "composer.lock",
]);

/** Extensions that are machine-generated lockfiles (covers *.lock, bun.lockb, Cargo.lock…). */
const LOCK_FILE_SUFFIXES = [".lock", ".lockb"];

/** Extensions that should never be rendered as text in the preview pane. */
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

export type FilePreviewBlockKind = "lockfile" | "binary" | "too-large";

export interface FilePreviewBlock {
  readonly kind: FilePreviewBlockKind;
  /** Short title for the blocked-preview panel. */
  readonly title: string;
  /** Human-readable explanation shown under the title. */
  readonly message: string;
}

function extensionOf(fileName: string): string {
  const idx = fileName.lastIndexOf(".");
  return idx === -1 ? "" : fileName.slice(idx).toLowerCase();
}

function isLockFile(fileName: string): boolean {
  const lower = fileName.toLowerCase();
  if (LOCK_FILE_BASENAMES.has(lower)) return true;
  return LOCK_FILE_SUFFIXES.some((suffix) => lower.endsWith(suffix));
}

function isBinaryFile(fileName: string): boolean {
  return BINARY_FILE_EXTENSIONS.has(extensionOf(fileName));
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * Decide whether a file may be opened in the preview.
 * Returns `null` when the file is allowed, otherwise a block reason.
 *
 * @param fileName basename of the file (e.g. "bun.lockb")
 * @param sizeBytes optional size from the tree entry stat; when unknown,
 * name-based rules still apply and only the size cap is skipped.
 */
export function getFilePreviewBlock(
  fileName: string,
  sizeBytes?: number,
): FilePreviewBlock | null {
  if (isLockFile(fileName)) {
    return {
      kind: "lockfile",
      title: "Lockfiles can't be previewed",
      message: `"${fileName}" is a generated lockfile — open it on your machine instead.`,
    };
  }
  if (isBinaryFile(fileName)) {
    return {
      kind: "binary",
      title: "Binary file",
      message: `"${fileName}" isn't a text file, so there's nothing to preview here.`,
    };
  }
  if (sizeBytes !== undefined && sizeBytes > MAX_FILE_PREVIEW_BYTES) {
    return {
      kind: "too-large",
      title: "File too large",
      message: `"${fileName}" is ${formatBytes(sizeBytes)} — previews are capped at ${formatBytes(MAX_FILE_PREVIEW_BYTES)}.`,
    };
  }
  return null;
}
