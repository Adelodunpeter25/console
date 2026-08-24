/**
 * Focused tests for the readFile tool (Task 1 of read-file-tool-implementation.md):
 * ceilings, exact resume points, normalization, classification, dangerous paths.
 */
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import { execSync } from "node:child_process";
import { readFileTool } from "../agent/src/tools/read/index.js";

console.log("Running readFile ceiling/normalization tests...");

const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "console-readfile-test-"));

const runTool = async (args: Record<string, unknown>) => {
  const parsed = readFileTool.inputSchema.parse(args);
  return readFileTool.execute(parsed) as Promise<Record<string, any>>;
};

const write = (name: string, content: string | Buffer) =>
  fs.writeFile(path.join(tempDir, name), content);

try {
  // 1. Empty file
  await write("empty.txt", "");
  const empty = await runTool({ path: "empty.txt", cwd: tempDir });
  assert.ok(empty.content[0].text.startsWith("File is empty"));
  assert.equal(empty.isError, undefined);
  assert.equal(empty.kind, "text");
  console.log("  ✅ empty file returns actionable message");

  // 2. startLine past EOF
  await write("ten.txt", Array.from({ length: 10 }, (_, i) => `line ${i + 1}`).join("\n") + "\n");
  const pastEof = await runTool({ path: "ten.txt", cwd: tempDir, startLine: 50 });
  assert.ok(pastEof.content[0].text.includes("Offset beyond EOF"));
  assert.ok(pastEof.content[0].text.includes("The file has 10 lines"));
  assert.equal(pastEof.totalLines, 10);
  assert.notEqual(pastEof.isError, true);
  console.log("  ✅ past-EOF returns total lines and recovery advice");

  // 3. startLine > endLine rejected clearly
  const inverted = await runTool({ path: "ten.txt", cwd: tempDir, startLine: 5, endLine: 2 });
  assert.equal(inverted.isError, true);
  assert.ok(inverted.content[0].text.includes("startLine (5) must be less than or equal to endLine (2)"));
  console.log("  ✅ startLine > endLine rejected");

  // 4. Line ceiling → exact resume line
  await write("big.txt", Array.from({ length: 5000 }, (_, i) => `L${i + 1}`).join("\n") + "\n");
  const capped = await runTool({ path: "big.txt", cwd: tempDir });
  assert.equal(capped.truncated, true);
  assert.equal(capped.resumeFrom, 2001);
  assert.ok(capped.content[0].text.includes("Resume with startLine=2001."));
  assert.ok(capped.content[0].text.includes("L2000"));
  assert.ok(!capped.content[0].text.includes("\n 2001: L2001"));
  console.log("  ✅ line ceiling truncates with exact resume point");

  // Resume actually works and lands on the promised line.
  const resumed = await runTool({ path: "big.txt", cwd: tempDir, startLine: 2001 });
  assert.ok(resumed.content[0].text.includes(": L2001\n"));
  // Ceilings apply to explicit reads too — 3000 remaining lines truncate again.
  assert.equal(resumed.truncated, true);
  assert.equal(resumed.resumeFrom, 4001);

  // A resume that fits under the ceiling reaches EOF and reports totals.
  const tail = await runTool({ path: "big.txt", cwd: tempDir, startLine: 3500 });
  assert.equal(tail.totalLines, 5000);
  assert.equal(tail.truncated, false);
  console.log("  ✅ resuming from stated startLine lands exactly there");

  // 5. Byte ceiling → resume names correct next line
  // 100 lines x ~4 KiB each; 128 KiB budget stops around line 33.
  const wideLine = `W${"x".repeat(4000)}`;
  await write(
    "wide.txt",
    Array.from({ length: 100 }, (_, i) => `${i + 1}:${wideLine}`).join("\n") + "\n",
  );
  const byteCapped = await runTool({ path: "wide.txt", cwd: tempDir });
  assert.equal(byteCapped.truncated, true);
  assert.ok(byteCapped.content[0].text.includes("truncated by the byte limit"));
  assert.ok(typeof byteCapped.resumeFrom === "number" && byteCapped.resumeFrom > 20 && byteCapped.resumeFrom < 60);
  // The emitted output must be bounded (~budget + one line of slack).
  const outBytes = Buffer.byteLength(byteCapped.content[0].text);
  assert.ok(outBytes < 160 * 1024, `output too large: ${outBytes}`);
  console.log(`  ✅ byte ceiling truncates with resume at line ${byteCapped.resumeFrom}`);

  // 6. Long line visibly truncated, no invalid UTF-8
  await write("long-line.txt", "A".repeat(5000) + "\n");
  const longLine = await runTool({ path: "long-line.txt", cwd: tempDir });
  assert.ok(longLine.content[0].text.includes("[line truncated at 2000 characters]"));
  // Cap is 2000 chars: more than that must not have survived.
  assert.ok(!longLine.content[0].text.includes("A".repeat(2500)));
  console.log("  ✅ long line truncated with marker");

  // 7. Multi-byte UTF-8 straddling a chunk boundary is not split
  // Lines of 10 "é" (2 bytes each) => 21-byte records; chunk boundaries land
  // inside multi-byte characters repeatedly across the ~48KB file.
  const line = "é".repeat(10);
  await write("utf8-boundary.txt", Array.from({ length: 2200 }, () => line).join("\n") + "\n");
  const utf8 = await runTool({ path: "utf8-boundary.txt", cwd: tempDir });
  const utf8Text: string = utf8.content[0].text;
  assert.ok(!utf8Text.includes("\uFFFD"), "replacement character found — UTF-8 was split");
  // 2000 emitted lines x 10 accents each, every one intact:
  const emittedAccentCount = (utf8Text.match(/é/g) ?? []).length;
  assert.equal(emittedAccentCount, 20000);
  console.log("  ✅ multi-byte chars across chunk boundaries not split");

  // 8. CRLF normalized, numbering intact
  await write("crlf.txt", "one\r\ntwo\r\nthree\r\n");
  const crlf = await runTool({ path: "crlf.txt", cwd: tempDir });
  const crlfLines = crlf.content[0].text.split("\n").filter((l: string) => /^\s*\d+: /.test(l));
  assert.equal(crlfLines.length, 3);
  assert.ok(!crlf.content[0].text.includes("\r"));
  assert.ok(crlfLines.some((l: string) => l.endsWith(": two")));
  console.log("  ✅ CRLF normalized with correct line numbers");

  // 9. BOM stripped
  await write("bom.txt", "\uFEFFfirst\n");
  const bom = await runTool({ path: "bom.txt", cwd: tempDir });
  assert.ok(bom.content[0].text.includes(": first"));
  assert.ok(!bom.content[0].text.includes("\uFEFF"));
  console.log("  ✅ BOM stripped");

  // 10. Dangerous paths blocked before I/O
  for (const p of ["/dev/zero", "/dev/urandom", "/proc/self/fd/1"]) {
    const blocked = await runTool({ path: p });
    assert.equal(blocked.isError, true, `${p} should be blocked`);
    assert.ok(blocked.content[0].text.includes("Refusing to read special device"));
  }
  console.log("  ✅ /dev/zero-style paths blocked");

  // 11. PDF magic → instruction message
  await write("doc.pdf", Buffer.from("%PDF-1.7 fake pdf body \u0000 more"));
  const pdf = await runTool({ path: "doc.pdf", cwd: tempDir });
  assert.ok(pdf.content[0].text.includes("This is a PDF"));
  assert.ok(pdf.content[0].text.includes("pdftotext"));
  assert.equal(pdf.kind, "pdf");
  assert.notEqual(pdf.isError, true);
  console.log("  ✅ PDF classified with extraction advice");

  // 12. Binary (NUL bytes) classified
  await write("blob.bin", Buffer.from([0x89, 0x00, 0x01, 0x00, 0x42]));
  const bin = await runTool({ path: "blob.bin", cwd: tempDir });
  assert.ok(bin.content[0].text.includes("Binary file"));
  assert.equal(bin.kind, "binary");
  console.log("  ✅ binary files classified");

  // 13. Image extension → metadata only
  await write("pic.png", Buffer.from([0x89, 0x50, 0x4e, 0x47]));
  const img = await runTool({ path: "pic.png", cwd: tempDir });
  assert.equal(img.kind, "image");
  assert.ok(img.content[0].text.includes("Image file"));
  console.log("  ✅ image extension classified");

  // 14. Base64 bounded
  await fs.writeFile(path.join(tempDir, "big.bin"), Buffer.alloc(300 * 1024, 7));
  const b64 = await runTool({ path: "big.bin", cwd: tempDir, encoding: "base64" });
  assert.equal(b64.truncated, true);
  assert.ok(b64.content[0].text.includes("NOT fully returned"));
  const b64Payload = b64.content[0].text.split("\n\n")[1] ?? "";
  assert.ok(Buffer.byteLength(b64Payload) < 200 * 1024);
  console.log("  ✅ base64 output bounded with explicit notice");

  // 15. Explicit endLine range still works
  const range = await runTool({ path: "ten.txt", cwd: tempDir, startLine: 3, endLine: 5 });
  assert.ok(range.content[0].text.includes(": line 3"));
  assert.ok(range.content[0].text.includes(": line 5"));
  assert.ok(!range.content[0].text.includes(": line 6"));
  // Range stopped before EOF: totals are unknown, but nothing is flagged wrong.
  assert.notEqual(range.isError, true);
  console.log("  ✅ explicit ranges work");

  // 16. Deep-offset read on a big file: byte budget spent scanning must not
  // produce a degenerate range or bogus resume pointer.
  const bigLog =
    Array.from({ length: 20000 }, (_, i) => `log ${i + 1} ${"y".repeat(60)}`).join("\n") + "\n";
  await write("big.log", bigLog); // ~1.5 MB, ~20k lines
  const deep = await runTool({ path: "big.log", cwd: tempDir, startLine: 19000 });
  assert.ok(
    deep.content[0].text.includes(`before reaching startLine 19000`),
    deep.content[0].text.slice(0, 200),
  );
  assert.ok(!deep.content[0].text.includes("lines 19000\u201318999"));
  assert.equal(deep.resumeFrom, undefined);
  console.log("  ✅ deep-offset scan-exhaustion returns clear branch message");

  // 17. Surrogate pairs never split at the char cap (emoji = 2 UTF-16 units)
  await write("emoji.txt", "😀".repeat(1999) + "🎉🎉🎉\n");
  const emoji = await runTool({ path: "emoji.txt", cwd: tempDir });
  const emojiText: string = emoji.content[0].text;
  for (const c of emojiText) {
    const cp = c.codePointAt(0)!;
    if (cp >= 0xd800 && cp <= 0xdfff) {
      throw new Error(`lone surrogate emitted: U+${cp.toString(16)}`);
    }
  }
  assert.ok(emojiText.includes("[line truncated at 2000 characters]"));
  assert.ok(!emojiText.includes("🎉"), "chars beyond cap should be dropped");
  console.log("  ✅ surrogate pairs never split by the line cap");

  // 18. FIFO rejection (named pipe hangs on open/read without writers)
  const fifoPath = path.join(tempDir, "pipe.fifo");
  await fs.rm(fifoPath, { force: true });
  await execSync(`mkfifo ${JSON.stringify(fifoPath)}`);
  const fifo = await runTool({ path: fifoPath });
  assert.equal(fifo.isError, true);
  assert.ok(fifo.content[0].text.includes("FIFO"));
  console.log("  ✅ FIFO rejected instead of hanging");

  // 19. Workspace containment: symlink escaping the project root is refused
  const outsideDir = await fs.mkdtemp(path.join(os.tmpdir(), "outside-"));
  const outsideFile = path.join(outsideDir, "secret.txt");
  await fs.writeFile(outsideFile, "top secret");
  const escapeLink = path.join(tempDir, "escape.txt");
  await fs.symlink(outsideFile, escapeLink);
  const escaped = await runTool({ path: "escape.txt", cwd: tempDir });
  assert.equal(escaped.isError, true);
  assert.ok(escaped.content[0].text.includes("outside the project directory"));
  // A legit in-root file still reads fine through the same check.
  const inside = await runTool({ path: "ten.txt", cwd: tempDir });
  assert.notEqual(inside.isError, true);
  console.log("  ✅ symlink escaping workspace root refused; in-root reads fine");

  // 20. base64 + line ranges rejected up front
  await assert.rejects(
    () => runTool({ path: "ten.txt", cwd: tempDir, encoding: "base64", startLine: 2 }),
    /not supported with encoding/,
  );
  console.log("  ✅ base64 combined with line ranges rejected clearly");

  console.log("readFile ceiling/normalization tests passed!");
} finally {
  await fs.rm(tempDir, { recursive: true, force: true });
}
