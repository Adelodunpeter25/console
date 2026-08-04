import {
  createRootRoute,
  createRoute,
  createRouter,
  createMemoryHistory,
  Outlet,
} from "@tanstack/react-router";
import { ChatPage } from "./pages/ChatPage";

const rootRoute = createRootRoute({
  component: () => <Outlet />,
});

const chatRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: ChatPage,
});

const routeTree = rootRoute.addChildren([chatRoute]);

/**
 * Memory-based router — appropriate for a Tauri desktop app where there is
 * no browser URL bar. Navigation is in-app only.
 */
export const router = createRouter({
  routeTree,
  history: createMemoryHistory({ initialEntries: ["/"] }),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
