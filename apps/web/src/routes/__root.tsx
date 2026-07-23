import { createRootRoute, Outlet } from "@tanstack/react-router";
import { ProjectSidebar } from "../components/sidebar/project-sidebar.js";

export const Route = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-background text-foreground antialiased flex">
      <ProjectSidebar />
      <main className="flex-1 h-screen flex flex-col relative overflow-hidden">
        <Outlet />
      </main>
    </div>
  ),
});
