/**
 * Project Management Service.
 * Business logic for listing projects, adding selected project folders, and scoping workspace context.
 */
import fs from "node:fs/promises";
import path from "node:path";
import type { ProjectInfo } from "../types/index.js";

const customProjects = new Set<string>();

export class ProjectService {
  /**
   * List all recent and selected project directories on the server.
   */
  async listProjects(): Promise<ProjectInfo[]> {
    const rootDir = process.cwd();
    const projectsMap = new Map<string, ProjectInfo>();

    // Add custom added projects
    for (const projPath of customProjects) {
      try {
        const stat = await fs.stat(projPath);
        projectsMap.set(projPath, {
          name: path.basename(projPath),
          path: projPath,
          lastModified: stat.mtimeMs,
        });
      } catch {
        // Ignored if custom project folder no longer exists
      }
    }

    // Auto-discover sibling workspace directories
    try {
      const entries = await fs.readdir(rootDir, { withFileTypes: true });
      for (const entry of entries) {
        if (entry.isDirectory() && !entry.name.startsWith(".")) {
          const fullPath = path.join(rootDir, entry.name);
          if (!projectsMap.has(fullPath)) {
            const stat = await fs.stat(fullPath);
            projectsMap.set(fullPath, {
              name: entry.name,
              path: fullPath,
              lastModified: stat.mtimeMs,
            });
          }
        }
      }
    } catch {
      // Ignored if readdir fails
    }

    // Always ensure current working directory is included
    if (!projectsMap.has(rootDir)) {
      projectsMap.set(rootDir, {
        name: path.basename(rootDir),
        path: rootDir,
        lastModified: Date.now(),
      });
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

    customProjects.add(resolvedPath);

    return {
      name: path.basename(resolvedPath),
      path: resolvedPath,
      lastModified: stat.mtimeMs,
    };
  }
}
