/**
 * Session Persistence Service wrapping SqliteSessionStorage.
 */
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";
import type { SessionHeader } from "../../../agent/src/types/index.js";
import type { CreateSessionDto, SessionDetailResponse, UpdateSessionDto } from "../types/index.js";

export class SessionService {
  private storage = new SqliteSessionStorage();

  listSessions(cwd?: string, projectId?: string): SessionHeader[] {
    return this.storage.listSessions({ cwd, projectId });
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
    });
  }

  getSession(sessionId: string): SessionDetailResponse | null {
    return this.storage.loadSession(sessionId);
  }

  updateSession(sessionId: string, dto: UpdateSessionDto): SessionHeader | null {
    if (dto.title) {
      this.storage.updateTitle(sessionId, dto.title);
    }
    if (dto.modelId) {
      const current = this.storage.loadSession(sessionId);
      const provider = dto.provider || current?.header.provider || "antigravity";
      this.storage.updateModel(sessionId, dto.modelId, provider);
    }

    const updated = this.storage.loadSession(sessionId);
    return updated ? updated.header : null;
  }

  deleteSession(sessionId: string): boolean {
    return this.storage.deleteSession(sessionId);
  }
}
