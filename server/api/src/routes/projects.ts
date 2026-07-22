/**
 * Project Management Routes (/api/projects/*).
 * Controller delegating business logic to ProjectService.
 */
import { Hono } from "hono";
import { ProjectService } from "../services/project.service.js";

export const projectRoutes = new Hono();
const projectService = new ProjectService();

/**
 * GET /api/projects — List all active and recent server project folders.
 */
projectRoutes.get("/projects", async (c) => {
  const projects = await projectService.listProjects();
  return c.json({
    success: true,
    data: projects,
  });
});

/**
 * POST /api/projects — Add a user-selected folder as a project workspace.
 */
projectRoutes.post("/projects", async (c) => {
  const body = await c.req.json<{ path: string }>();
  if (!body.path) {
    return c.json({ success: false, error: "Field 'path' is required." }, 400);
  }

  try {
    const project = await projectService.addProject(body.path);
    return c.json({
      success: true,
      data: project,
    });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 400);
  }
});
