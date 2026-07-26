/**
 * React 19 type compatibility overrides.
 *
 * react-markdown v10 and sonner v2 export components whose inferred return
 * types don't satisfy React 19's stricter JSX element type. These overrides
 * relax the component types so they can be used in JSX without error.
 */
declare module "react-markdown" {
  import type { ComponentType } from "react";
  const ReactMarkdown: ComponentType<Record<string, unknown>>;
  export default ReactMarkdown;
}

declare module "sonner" {
  import type { ComponentType } from "react";
  export const Toaster: ComponentType<Record<string, unknown>>;
}
