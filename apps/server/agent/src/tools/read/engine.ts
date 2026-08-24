/**
 * Read-engine primitives shared by filesystem tools.
 *
 * Pure mechanism (streaming line feeder) and policy (safety ceilings, content
 * classification) with no knowledge of the agent tool contract. The readFile
 * tool wires these into its schema/execute; Task 2's write guards reuse them.
 */
import * as fs from "node:fs/promises";
import * as path from "node:path";
import { StringDecoder } from "node:string_decoder";

// ---------------------------------------------------------------------------
// Ceilings (independent; a read stops as soon as any one is reached)
// ---------------------------------------------------------------------------
export const MAX_LINES = 2000; // controls normal large source files
export const MAX_BYTES = 128 * 1024; // controls wide or binary-like content
export const MAX_LINE_CHARS = 2000; // controls minified JS and long log lines
const CHUNK_SIZE = 16 * 1024; // streaming chunk size
const SNIFF_SIZE = 8 * 1024; // bytes inspected for binary/PDF detection

export type FileKind = "text" | "binary" | "pdf" | "image" | "notebook";

export interface ReadMetadata {
  startLine?: number;
  endLine?: number;
  totalLines?: number;
  truncated?: boolean;
  resumeFrom?: number;
  sizeBytes?: number;
  mtimeMs?: number;
  kind?: FileKind;
}

/** Block special devices and unbounded pseudo-files before any I/O. */
export function isBlockedPath(p: string): boolean {
  const lower = p.toLowerCase();
  if (lower === "/proc" || lower.startsWith("/proc/")) return true;
  if (lower === "/sys" || lower.startsWith("/sys/")) return true;
  return /^\/dev\/(zero|urandom|random|mem|kmem|port)$/.test(lower);
}

const IMAGE_EXTENSIONS = new Set([
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".webp",
  ".bmp",
  ".ico",
  ".tiff",
]);

function fileKindForExtension(ext: string): FileKind | null {
  const lower = ext.toLowerCase();
  if (IMAGE_EXTENSIONS.has(lower)) return "image";
  if (lower === ".ipynb") return "notebook";
  return null;
}

/**
 * Classify a file before reading it as text. Reads a small head buffer and
 * combines magic bytes / NUL detection with extension-based declarations.
 */
export async function classifyFile(
  filePath: string,
  fh: fs.FileHandle,
  size: number,
): Promise<{ kind: FileKind; looksBinary: boolean }> {
  const sniff = Buffer.alloc(Math.min(size, SNIFF_SIZE));
  const { bytesRead: sniffed } = await fh.read(sniff, 0, sniff.length, 0);
  const head = sniff.subarray(0, sniffed);
  const declaredKind = fileKindForExtension(path.extname(filePath));
  if (sniffed >= 5 && head.subarray(0, 5).toString("latin1") === "%PDF-") {
    return { kind: "pdf", looksBinary: true };
  }
  if (declaredKind) return { kind: declaredKind, looksBinary: false };
  return {
    kind: "text",
    looksBinary: head.includes(0),
  };
}

/**
 * Incremental line feeder over an open file handle.
 *
 * Streams fixed-size chunks, decodes UTF-8 safely across chunk boundaries
 * (StringDecoder), normalizes CRLF / lone-CR endings (including a CR that
 * straddles a chunk boundary), strips a leading BOM, and never loads more
 * than one chunk plus the current line into memory.
 *
 * Line counting follows `cat -n` semantics: a trailing newline does not
 * create an extra empty line.
 */
export class StreamingLineFeeder {
  private readonly decoder = new StringDecoder("utf-8");
  private heldCr = false; // saw "\r" as last decoded char; may merge with "\n"
  private lineBuf = ""; // characters of the line currently being assembled
  private lineBufChars = 0;
  private strippedBom = false;
  private currentLineTruncated = false;

  /** Number of fully completed lines seen so far (== last completed line no). */
  completedLines = 0;
  bytesRead = 0;
  eof = false;
  /** Set by the consumer to stop all feeding and counting immediately. */
  halted = false;
  /** Consumer callback; may call halt() to stop further consumption. */
  onLineComplete?: (line: string, originalCharCount: number, truncated: boolean) => void;

  constructor(private readonly fh: fs.FileHandle) {}

  halt(): void {
    this.halted = true;
    this.lineBuf = ""; // discard any partially assembled line
    this.lineBufChars = 0;
  }

  /** Read the next chunk and feed it through. Returns false once EOF is hit. */
  async readNextChunk(): Promise<boolean> {
    if (this.eof || this.halted) return false;
    const buffer = Buffer.alloc(CHUNK_SIZE);
    const { bytesRead } = await this.fh.read(buffer, 0, CHUNK_SIZE, this.bytesRead);
    this.bytesRead += bytesRead;
    let text = bytesRead > 0 ? this.decoder.write(buffer.subarray(0, bytesRead)) : "";
    if (bytesRead < CHUNK_SIZE) {
      text += this.decoder.end();
      this.eof = true;
    }
    this.consume(text);
    return !this.eof;
  }

  /** Complete the in-progress line (if any) at EOF. */
  finishAtEof(): void {
    if (this.heldCr) {
      this.finishLine(); // trailing "\r" terminates the final line
      this.heldCr = false;
    }
    if (this.lineBuf.length > 0) this.finishLine();
  }

  private consume(text: string): void {
    let s = (this.heldCr ? "\r" : "") + text;
    this.heldCr = false;
    if (!this.strippedBom) {
      this.strippedBom = true;
      if (s.startsWith("\uFEFF")) s = s.slice(1);
    }
    let i = 0;
    while (i < s.length) {
      if (this.halted) return;
      const ch = s[i]!;
      if (ch === "\r") {
        if (i === s.length - 1 && !this.eof) {
          this.heldCr = true; // might be "\r\n" split across chunks — hold it
          return;
        }
        this.finishLine();
        i += s[i + 1] === "\n" ? 2 : 1;
        continue;
      }
      if (ch === "\n") {
        this.finishLine();
        i += 1;
        continue;
      }
      // Append whole code points (never splitting surrogate pairs), counting
      // astral characters as 2 UTF-16 units toward MAX_LINE_CHARS. Characters
      // beyond the per-line cap are dropped; the flag tells the consumer to
      // render the truncation marker.
      const codePoint = s.codePointAt(i)!;
      const width = codePoint > 0xffff ? 2 : 1;
      if (this.lineBufChars + width <= MAX_LINE_CHARS) {
        this.lineBuf += String.fromCodePoint(codePoint);
        this.lineBufChars += width;
      } else {
        // Character does not fit under the cap — drop it without splitting.
        this.currentLineTruncated = true;
      }
      i += width;
    }
  }

  private finishLine(): void {
    if (this.halted) return;
    this.completedLines += 1;
    this.onLineComplete?.(this.lineBuf, this.lineBufChars, this.currentLineTruncated);
    this.currentLineTruncated = false;
    this.lineBuf = "";
    this.lineBufChars = 0;
  }
}
