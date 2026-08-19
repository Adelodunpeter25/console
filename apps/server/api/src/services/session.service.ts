/**
 * Session Persistence Service wrapping SqliteSessionStorage.
 */
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";
import type { SessionHeader } from "@console/types";
import type { CreateSessionDto, SessionDetailResponse, UpdateSessionDto } from "../types/index.js";
import { RunService } from "./run.service.js";

export class SessionService {
  private storage = new SqliteSessionStorage();

  listSessions(cwd?: string, projectId?: string, onlyDeleted?: boolean): SessionHeader[] {
    return this.storage.listSessions({ cwd, projectId, onlyDeleted });
  }

  createSession(dto: CreateSessionDto): SessionHeader {
    const cwd = dto.cwd || process.cwd();
    const modelId = dto.modelId || "gemini-2.5-pro";
    const provider = dto.provider || "antigravity";
    const title = dto.title || "New Session";

    let projectId = dto.projectId;
    if (!projectId) {
      const project = this.storage.getProjectByDir(cwd);
      if (project) {
        projectId = project.id;
      }
    }

    return this.storage.createSession({
      title,
      cwd,
      projectId,
      modelId,
      provider,
      approvalMode: dto.approvalMode,
    });
  }

  getSession(
    sessionId: string,
    options: { limit?: number; before?: number } = { limit: 50 },
  ): SessionDetailResponse | null {
    const session = this.storage.loadSessionPage(sessionId, options);
    if (!session) return null;

    if (!RunService.isRunActive(sessionId)) {
      if (session.header.status === "working" || session.header.status === "needs_attention") {
        this.storage.updateSessionStatus(sessionId, "done");
        session.header.status = "done";
      }
    }

    return session;
  }

  updateSession(sessionId: string, dto: UpdateSessionDto): SessionHeader | null {
    if (dto.title) {
      this.storage.updateTitle(sessionId, dto.title);
    }
    if (dto.cwd) {
      this.storage.updateCwd(sessionId, dto.cwd);
    }
    if (dto.modelId) {
      const current = this.storage.loadSession(sessionId);
      const provider = dto.provider || current?.header.provider || "antigravity";
      this.storage.updateModel(sessionId, dto.modelId, provider);
    }
    if (dto.approvalMode) {
      this.storage.updateApprovalMode(sessionId, dto.approvalMode);
    }

    const updated = this.storage.loadSession(sessionId);
    return updated ? updated.header : null;
  }

  deleteSession(sessionId: string): boolean {
    return this.storage.deleteSession(sessionId);
  }

  restoreSession(sessionId: string): boolean {
    return this.storage.restoreSession(sessionId);
  }

  permanentlyDeleteSession(sessionId: string): boolean {
    return this.storage.permanentlyDeleteSession(sessionId);
  }
}
