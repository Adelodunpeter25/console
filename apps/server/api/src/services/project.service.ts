/**
 * Project Management Service.
 * Business logic for listing projects, adding selected project folders, and scoping workspace context.
 */
import fs from "node:fs/promises";
import path from "node:path";
import type { ProjectInfo } from "../types/index.js";
import { SqliteSessionStorage } from "../../../agent/src/session/storage.js";

export class ProjectService {
  private storage = new SqliteSessionStorage();

  /**
   * List all recent and selected project directories on the server.
   */
  async listProjects(): Promise<ProjectInfo[]> {
    const rootDir = process.cwd();
    const projectsMap = new Map<string, ProjectInfo>();

    // Load persistent projects from SQLite database
    try {
      const dbProjects = this.storage.listProjects();
      for (const proj of dbProjects) {
        try {
          const stat = await fs.stat(proj.path);
          projectsMap.set(proj.path, {
            id: proj.id,
            name: proj.name,
            path: proj.path,
            createdAt: proj.createdAt,
            updatedAt: stat.mtimeMs,
          });
        } catch {
          // Ignored if custom project folder no longer exists
        }
      }
    } catch (e) {
      console.error("Failed to load projects from DB:", e);
    }

    return Array.from(projectsMap.values());
  }

  /**
   * Add a user-selected directory path as a project folder.
   */
  async addProject(folderPath: string): Promise<ProjectInfo> {
    const resolvedPath = path.resolve(folderPath);
    const stat = await fs.stat(resolvedPath);

    if (!stat.isDirectory()) {
      throw new Error(`Path '${folderPath}' is not a valid directory.`);
    }

    return this.storage.createProject({
      name: path.basename(resolvedPath),
      dir: resolvedPath,
    });
  }
}
