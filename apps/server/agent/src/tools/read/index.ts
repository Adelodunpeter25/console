import * as fs from "node:fs/promises";
import * as path from "node:path";
import { z } from "zod";
import type { AgentTool } from "../../types/index.js";
import { pathString } from "../../service/tool-input.js";
import {
  classifyFile,
  isBlockedPath,
  MAX_BYTES,
  MAX_LINE_CHARS,
  MAX_LINES,
  ReadMetadata,
  StreamingLineFeeder,
} from "./engine.js";

const BASE64_RAW_LIMIT = 96 * 1024; // ~128 KiB of base64 output

function metadataResult(meta: ReadMetadata, ...paragraphs: string[]) {
  return {
    content: [{ type: "text" as const, text: paragraphs.join("\n") }],
    ...meta,
  };
}

const inputSchema = z.object({
  path: pathString(
    'Required filesystem path to the file to read. Use "." only when a directory target is intended.',
  ),
  cwd: z
    .string()
    .optional()
    .describe("Base directory for relative paths. Defaults to process.cwd()."),
  startLine: z
    .number()
    .int()
    .positive()
    .optional()
    .describe("First line to read (1-indexed, inclusive). Omit to read from the beginning."),
  endLine: z
    .number()
    .int()
    .positive()
    .optional()
    .describe("Last line to read (1-indexed, inclusive). Omit to read until a safety limit."),
  encoding: z
    .enum(["utf-8", "base64"])
    .optional()
    .default("utf-8")
    .describe("File encoding. Use base64 for binary files."),
});

type Input = z.infer<typeof inputSchema>;

export const readFileTool: AgentTool<typeof inputSchema> = {
  name: "readFile",
  description: `Read the contents of a file at the given path.
Supports optional line range selection for large files.
Lines are returned with their 1-indexed line numbers prefixed (e.g. "  42: content"), matching cat -n numbering.
Reads are capped at ${MAX_LINES} lines / ${MAX_BYTES / 1024} KiB / ${MAX_LINE_CHARS} chars per line; every truncation result states the exact startLine to resume from.
Always prefer reading specific line ranges for large files to avoid wasting tokens.
For directories, use listDir instead.`,
  inputSchema,
  execute: async (args: Input, _signal?: AbortSignal): Promise<unknown> => {
    const filePath = path.resolve(args.cwd ?? process.cwd(), args.path);

    if (isBlockedPath(filePath)) {
      return {
        content: [
          {
            type: "text",
            text: `Error: Refusing to read special device or pseudo-file: ${filePath}`,
          },
        ],
        isError: true,
      };
    }

    let fh: fs.FileHandle;
    try {
      fh = await fs.open(filePath, "r");
    } catch (err: unknown) {
      return handleOpenError(err, filePath);
    }

    try {
      const stat = await fh.stat();

      if (stat.isDirectory()) {
        return metadataResult(
          { kind: "binary", sizeBytes: stat.size },
          `"${filePath}" is a directory. Use listDir to browse directories.`,
        );
      }

      const baseMeta = { sizeBytes: stat.size, mtimeMs: stat.mtimeMs };

      if (args.encoding === "base64") {
        return await readBase64(filePath, fh, stat.size);
      }

      if (stat.size === 0) {
        return metadataResult({ ...baseMeta, kind: "text", truncated: false }, `File is empty: ${filePath}`);
      }

      // --- Classify content -------------------------------------------------
      const { kind: declaredKind, looksBinary } = await classifyFile(filePath, fh, stat.size);

      if (declaredKind === "pdf") {
        return metadataResult(
          { ...baseMeta, kind: "pdf", truncated: false },
          `This is a PDF (${stat.size} bytes). Use pdftotext or a PDF-capable tool to extract its contents.`,
        );
      }
      if (declaredKind === "image") {
        return metadataResult(
          { ...baseMeta, kind: "image", truncated: false },
          `Image file (${path.extname(filePath)}, ${stat.size} bytes). Binary content cannot be displayed as text.`,
        );
      }
      if (looksBinary && declaredKind !== "notebook") {
        return metadataResult(
          { ...baseMeta, kind: "binary", truncated: false },
          `Binary file (${path.extname(filePath) || "no extension"}, ${stat.size} bytes). Cannot display as text.`,
        );
      }
      // SVG (XML text) and notebooks (JSON text) fall through to the normal
      // text ceilings under their declared kinds.

      return await readText({
        filePath,
        fh,
        sizeBytes: stat.size,
        mtimeMs: stat.mtimeMs,
        kind: declaredKind ?? "text",
        startLine: args.startLine,
        endLine: args.endLine,
      });
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException;
      return {
        content: [{ type: "text", text: `Error reading file: ${error.message}` }],
        isError: true,
      };
    } finally {
      await fh.close();
    }
  },
};

async function readBase64(filePath: string, fh: fs.FileHandle, size: number): Promise<unknown> {
  const limited = Math.min(size, BASE64_RAW_LIMIT);
  const buf = Buffer.alloc(limited);
  await fh.read(buf, 0, limited, 0);
  const b64 = buf.toString("base64");
  const truncated = size > limited;
  const notice = truncated
    ? `\n\nTruncated at ${limited} of ${size} bytes. The binary content was NOT fully returned.`
    : "";
  return metadataResult(
    { kind: "binary", truncated, sizeBytes: size },
    `File: ${filePath}\nEncoding: base64${truncated ? " (partial)" : ""}\nSize: ${size} bytes\n\n${b64}${notice}`,
  );
}

function handleOpenError(err: unknown, filePath: string): unknown {
  const error = err as NodeJS.ErrnoException;
  if (error.code === "ENOENT") {
    return {
      content: [{ type: "text", text: `Error: File not found: ${filePath}` }],
      isError: true,
    };
  }
  if (error.code === "EACCES") {
    return {
      content: [{ type: "text", text: `Error: Permission denied reading: ${filePath}` }],
      isError: true,
    };
  }
  return {
    content: [{ type: "text", text: `Error reading file: ${error.message}` }],
    isError: true,
  };
}

interface TextReadOptions {
  filePath: string;
  fh: fs.FileHandle;
  sizeBytes: number;
  mtimeMs: number;
  kind: ReadMetadata["kind"];
  startLine?: number;
  endLine?: number;
}

async function readText(opts: TextReadOptions): Promise<unknown> {
  const { filePath, fh, sizeBytes, mtimeMs, kind } = opts;
  const start = opts.startLine ?? 1;
  const requestedEnd = opts.endLine;

  if (requestedEnd !== undefined && start > requestedEnd) {
    return {
      content: [
        {
          type: "text",
          text: `startLine (${start}) must be less than or equal to endLine (${requestedEnd}).`,
        },
      ],
      isError: true,
    };
  }

  // A user-supplied range narrows what we emit but never bypasses the caps.
  const emitCap = Math.min(MAX_LINES, requestedEnd !== undefined ? requestedEnd - start + 1 : MAX_LINES);

  const feeder = new StreamingLineFeeder(fh);
  const lines: string[] = [];
  let stoppedByLines = false;
  let stoppedByEndLine = false;
  let stoppedByBytes = false;
  let byteBudgetHit = false;

  feeder.onLineComplete = (line, _originalCharCount, lineWasTruncated) => {
    if (feeder.halted) return; // safety: no emissions after a stop condition
    const lineNum = feeder.completedLines;

    if (requestedEnd !== undefined && lineNum > requestedEnd) {
      // One line past the requested range proves more content exists. Un-count
      // this speculative line so the resume point lands exactly on it.
      stoppedByEndLine = true;
      feeder.completedLines -= 1;
      feeder.halt();
      return;
    }
    if (lineNum < start) return; // scanning toward startLine

    if (lineWasTruncated) {
      lines.push(`${line} [line truncated at ${MAX_LINE_CHARS} characters]`);
    } else {
      lines.push(line);
    }
    if (lines.length >= emitCap) {
      stoppedByLines = true;
      feeder.halt();
    }
  };

  for (;;) {
    if (feeder.bytesRead >= MAX_BYTES) {
      byteBudgetHit = true;
      break;
    }
    const more = await feeder.readNextChunk();
    if (feeder.halted) break;
    if (!more) break;
  }
  if (!feeder.halted && byteBudgetHit) stoppedByBytes = true;
  // When halted by a ceiling, the partially assembled line is discarded and
  // will be re-read from the resume line — never emitted half a line.
  feeder.finishAtEof();

  const eofReached = feeder.eof && !feeder.halted;
  const truncated = stoppedByLines || stoppedByEndLine || stoppedByBytes;
  const lastCompletedLine = feeder.completedLines;
  const resumeFrom = lastCompletedLine + 1;

  // Past-EOF recovery (expected condition, not an error).
  if (lines.length === 0 && !truncated && eofReached) {
    return metadataResult(
      { startLine: start, totalLines: lastCompletedLine, truncated: false, sizeBytes, mtimeMs, kind },
      `Offset beyond EOF. The file has ${lastCompletedLine} lines. Try a smaller startLine.`,
    );
  }

  const firstEmitted = start;
  const lastEmitted = firstEmitted + lines.length - 1;

  const rangeDescription =
    !truncated && eofReached
      ? firstEmitted === 1
        ? `all ${lastCompletedLine} lines`
        : `lines ${firstEmitted}\u2013${lastEmitted} of ${lastCompletedLine}`
      : `lines ${firstEmitted}\u2013${lastEmitted}`;

  const defaultNotes: string[] = [];
  if (opts.endLine !== undefined && opts.startLine === undefined) {
    defaultNotes.push("startLine defaulted to 1 because endLine was provided");
  }
  if (opts.startLine !== undefined && opts.endLine === undefined) {
    defaultNotes.push("endLine defaulted to the end of the file");
  }

  let resumeNotice = "";
  if (stoppedByLines || stoppedByEndLine) {
    resumeNotice = `Output truncated at line ${lastCompletedLine}. Resume with startLine=${resumeFrom}.`;
  } else if (stoppedByBytes) {
    resumeNotice = `Output truncated by the byte limit. Resume with startLine=${resumeFrom}.`;
  }

  const header = [
    `File: ${filePath}`,
    `Showing: ${rangeDescription}${truncated ? " (truncated)" : ""}`,
    `Size: ${sizeBytes} bytes`,
    ...(defaultNotes.length > 0 ? [`Defaults applied: ${defaultNotes.join("; ")}`] : []),
    ...(resumeNotice ? [resumeNotice] : []),
    "",
  ].join("\n");

  const lineWidth = String(lastEmitted).length;
  const numbered = lines
    .map((line, i) => `${String(firstEmitted + i).padStart(lineWidth, " ")}: ${line}`)
    .join("\n");

  return {
    content: [{ type: "text", text: header + numbered }],
    startLine: firstEmitted,
    endLine: lastEmitted,
    truncated,
    kind,
    sizeBytes,
    mtimeMs,
    ...(eofReached ? { totalLines: lastCompletedLine } : {}),
    ...(truncated ? { resumeFrom } : {}),
  };
}
