/**
 * Functional tests for local agent tools.
 * Operates on temporary local files — zero external network/API credits.
 */
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import {
  batchWriteTool,
  editFileTool,
  globTool,
  grepTool,
  listDirTool,
  readFileTool,
  writeFileTool,
} from "../agent/src/tools/index.js";

console.log("Running Local Tools functional tests...");

const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), "console-tools-test-"));

try {
  // Helper to parse input through Zod schema (populates default values) and execute
  const runTool = async (tool: any, args: Record<string, unknown>) => {
    const parsed = tool.inputSchema.parse(args);
    return tool.execute(parsed);
  };

  // 1. Write file
  const testFilePath = path.join(tempDir, "hello.txt");
  const writeRes = (await runTool(writeFileTool, {
    path: testFilePath,
    content: "Line 1: Hello\nLine 2: World\nLine 3: Test",
  })) as { content: Array<{ text: string }> };

  assert.ok(writeRes.content[0]?.text.includes("Written:"));
  console.log("  ✅ writeFile tool");

  // 2. Read file
  const readRes = (await runTool(readFileTool, {
    path: testFilePath,
    startLine: 1,
    endLine: 2,
  })) as { content: Array<{ text: string }> };

  assert.ok(readRes.content[0]?.text.includes("Line 1: Hello"));
  assert.ok(readRes.content[0]?.text.includes("Line 2: World"));
  assert.ok(!readRes.content[0]?.text.includes("Line 3: Test"));
  console.log("  ✅ readFile tool (with line ranges)");

  // 3. Edit file
  const editRes = (await runTool(editFileTool, {
    path: testFilePath,
    oldContent: "Line 2: World",
    newContent: "Line 2: Universe",
  })) as { content: Array<{ text: string }> };

  assert.ok(editRes.content[0]?.text.includes("Edited:"));
  const readEdited = (await runTool(readFileTool, { path: testFilePath })) as {
    content: Array<{ text: string }>;
  };
  assert.ok(readEdited.content[0]?.text.includes("Line 2: Universe"));
  console.log("  ✅ editFile tool");

  // 4. Batch write
  const fileA = path.join(tempDir, "sub", "a.txt");
  const fileB = path.join(tempDir, "sub", "b.txt");
  const batchRes = (await runTool(batchWriteTool, {
    files: [
      { path: fileA, content: "File A content" },
      { path: fileB, content: "File B content" },
    ],
  })) as { content: Array<{ text: string }> };

  assert.ok(batchRes.content[0]?.text.includes("2/2 files written successfully"));
  console.log("  ✅ batchWrite tool");

  // 5. List dir
  const listRes = (await runTool(listDirTool, {
    path: tempDir,
    recursive: true,
  })) as { content: Array<{ text: string }> };

  assert.ok(listRes.content[0]?.text.includes("hello.txt"));
  assert.ok(listRes.content[0]?.text.includes("a.txt"));
  console.log("  ✅ listDir tool");

  // 6. Glob search (using fff-node)
  const globRes = (await runTool(globTool, {
    pattern: "**/*.txt",
    cwd: tempDir,
  })) as { content: Array<{ text: string }> };

  assert.ok(globRes.content[0]?.text.includes("hello.txt"));
  assert.ok(globRes.content[0]?.text.includes("a.txt"));
  console.log("  ✅ glob tool (fff-powered)");

  // 7. Grep search (using fff-node)
  const grepRes = (await runTool(grepTool, {
    pattern: "Universe",
    path: tempDir,
  })) as { content: Array<{ text: string }> };

  assert.ok(grepRes.content[0]?.text.includes("Universe"));
  console.log("  ✅ grep tool (fff-powered)");
} finally {
  await fs.rm(tempDir, { recursive: true, force: true });
}

console.log("Local Tools functional tests passed!\n");
