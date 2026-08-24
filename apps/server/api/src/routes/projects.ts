/**
 * Project Management Routes (/api/projects/*).
 * Controller delegating business logic to ProjectService.
 */
import { Hono } from "hono";
import { ProjectService } from "@/api/src/services/project.service.js";

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

/**
 * DELETE /api/projects/:id — Remove a project workspace by ID.
 */
projectRoutes.delete("/projects/:id", async (c) => {
  const projectId = c.req.param("id");
  try {
    const deleted = await projectService.deleteProject(projectId);
    if (!deleted) {
      return c.json({ success: false, error: `Project '${projectId}' not found.` }, 404);
    }
    return c.json({
      success: true,
      data: { id: projectId, deleted: true },
    });
  } catch (err) {
    const errorMsg = err instanceof Error ? err.message : String(err);
    return c.json({ success: false, error: errorMsg }, 500);
  }
});
