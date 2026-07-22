/**
 * Hono Application Factory & Route Mounting.
 */
import { Hono } from "hono";
import { cors } from "hono/cors";
import { logger } from "hono/logger";
import { authRoutes } from "./routes/auth.js";
import { fsRoutes } from "./routes/fs.js";
import { projectRoutes } from "./routes/projects.js";
import { providerRoutes } from "./routes/providers.js";
import { runRoutes } from "./routes/run.js";
import { sessionRoutes } from "./routes/sessions.js";

export function createApiApp(): Hono {
  const app = new Hono();

  // Middleware
  app.use("*", cors({ origin: "*", allowMethods: ["GET", "POST", "PATCH", "DELETE", "OPTIONS"] }));
  app.use("*", logger());

  // Health check
  app.get("/health", (c) =>
    c.json({ status: "ok", engine: "console-agent", timestamp: Date.now() }),
  );

  // Mount API Sub-routers under /api
  const api = new Hono();
  api.route("/auth", authRoutes);
  api.route("/fs", fsRoutes);
  api.route("/", projectRoutes);
  api.route("/", providerRoutes);
  api.route("/", sessionRoutes);
  api.route("/", runRoutes);

  app.route("/api", api);

  // Global 404 handler
  app.notFound((c) => c.json({ success: false, error: `Route '${c.req.path}' not found.` }, 404));

  // Global Error handler
  app.onError((err, c) => {
    console.error("API Error:", err);
    return c.json(
      {
        success: false,
        error: err instanceof Error ? err.message : "Internal Server Error",
      },
      500,
    );
  });

  return app;
}
