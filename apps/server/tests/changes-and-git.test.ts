import { describe, expect, it } from "bun:test";
import { SqliteSessionStorage } from "@/agent/src/session/storage.js";
import { createApiApp } from "@/api/src/app.js";
import { GitService } from "@/api/src/services/git.service.js";
import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";

describe("Session File Changes & Git Endpoints", () => {
  it("records and retrieves session file changes in sqlite storage", () => {
    const storageDir = fs.mkdtempSync(path.join(os.tmpdir(), "console-test-storage-"));
    const storage = new SqliteSessionStorage({ storageDir });

    const session = storage.createSession({
      title: "Test Session",
      cwd: process.cwd(),
      modelId: "claude-opus-4-6-thinking",
      provider: "antigravity",
    });

    expect(storage.getSessionFileChanges(session.id)).toEqual([]);

    storage.recordFileChange(session.id, {
      path: "src/main.rs",
      status: "modified",
      additions: 15,
      deletions: 3,
      turnIndex: 1,
      updatedAt: Date.now(),
    });

    storage.recordFileChange(session.id, {
      path: "src/lib.rs",
      status: "added",
      additions: 40,
      deletions: 0,
      turnIndex: 1,
      updatedAt: Date.now(),
    });

    const changes = storage.getSessionFileChanges(session.id);
    expect(changes.length).toBe(2);

    const mainChange = changes.find((c) => c.path === "src/main.rs");
    expect(mainChange).toBeDefined();
    expect(mainChange?.status).toBe("modified");
    expect(mainChange?.additions).toBe(15);
    expect(mainChange?.deletions).toBe(3);

    // Upsert on conflict
    storage.recordFileChange(session.id, {
      path: "src/main.rs",
      status: "modified",
      additions: 25,
      deletions: 5,
      turnIndex: 2,
      updatedAt: Date.now(),
    });

    const updatedChanges = storage.getSessionFileChanges(session.id);
    expect(updatedChanges.length).toBe(2);
    const updatedMain = updatedChanges.find((c) => c.path === "src/main.rs");
    expect(updatedMain?.additions).toBe(25);
    expect(updatedMain?.deletions).toBe(5);

    storage.close();
    fs.rmSync(storageDir, { recursive: true, force: true });
  });

  it("handles GET /api/sessions/:id/changes via HTTP", async () => {
    const app = createApiApp();

    // Create session
    const createRes = await app.request("/api/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        title: "HTTP Changes Test",
        cwd: process.cwd(),
      }),
    });
    expect(createRes.status).toBe(200);
    const createBody = (await createRes.json()) as any;
    const sessionId = createBody.data.id;

    // Fetch initial changes
    const getRes = await app.request(`/api/sessions/${sessionId}/changes`);
    expect(getRes.status).toBe(200);
    const getBody = (await getRes.json()) as any;
    expect(getBody.success).toBe(true);
    expect(Array.isArray(getBody.data)).toBe(true);
  });

  it("computes git status with line additions/deletions and returns git diff", async () => {
    const gitService = new GitService();
    const status = await gitService.getGitStatus(process.cwd());

    expect(typeof status.branch).toBe("string");
    expect(typeof status.clean).toBe("boolean");
    expect(Array.isArray(status.files)).toBe(true);

    for (const f of status.files) {
      expect(typeof f.path).toBe("string");
      expect(typeof f.status).toBe("string");
      expect(typeof f.staged).toBe("boolean");
      if (f.additions !== undefined) {
        expect(typeof f.additions).toBe("number");
      }
      if (f.deletions !== undefined) {
        expect(typeof f.deletions).toBe("number");
      }
    }

    const diff = await gitService.getDiff(process.cwd());
    expect(typeof diff).toBe("string");

    // HTTP /api/git/diff test
    const app = createApiApp();
    const diffRes = await app.request(`/api/git/diff?cwd=${encodeURIComponent(process.cwd())}`);
    expect(diffRes.status).toBe(200);
    const diffBody = (await diffRes.json()) as any;
    expect(diffBody.success).toBe(true);
    expect(typeof diffBody.data.diff).toBe("string");
  });
});
