import React from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { ConsoleApiProvider } from "@console/api";
import { routeTree } from "./routeTree.gen.js";
import "./index.css";

const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ConsoleApiProvider baseUrl="http://localhost:3000">
      <RouterProvider router={router} />
    </ConsoleApiProvider>
  </React.StrictMode>,
);
