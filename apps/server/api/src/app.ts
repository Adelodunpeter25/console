/**
 * Hono Application Factory & Route Mounting.
 */
import { Hono } from "hono";
import { cors } from "hono/cors";
import { logger } from "hono/logger";
import { authRoutes } from "./routes/auth.js";
import { assistRoutes } from "./routes/assist.js";
import { configRoutes } from "./routes/config.js";
import { fsRoutes } from "./routes/fs.js";
import { notificationRoutes } from "./routes/notifications.js";
import { projectRoutes } from "./routes/projects.js";
import { providerRoutes } from "./routes/providers.js";
import { runRoutes } from "./routes/run.js";
import { sessionRoutes } from "./routes/sessions.js";
import { modelFavoriteRoutes } from "./routes/model-favorites.js";
import { usageRoutes } from "./routes/usage.js";

import { gitRoutes } from "./routes/git.js";

export function createApiApp(): Hono {
  const app = new Hono();

  // Middleware
  app.use("*", cors({ origin: "*", allowMethods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"] }));
  app.use("*", logger());

  // Health check
  app.get("/health", (c) =>
    c.json({ status: "ok", engine: "console-agent", timestamp: Date.now() }),
  );

  // Mount API Sub-routers under /api
  const api = new Hono();
  api.route("/auth", authRoutes);
  api.route("/", assistRoutes);
  api.route("/", configRoutes);
  api.route("/fs", fsRoutes);
  api.route("/git", gitRoutes);
  api.route("/", notificationRoutes);
  api.route("/", projectRoutes);
  api.route("/", providerRoutes);
  api.route("/", modelFavoriteRoutes);
  api.route("/", sessionRoutes);
  api.route("/", runRoutes);
  api.route("/", usageRoutes);

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
