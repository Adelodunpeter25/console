/**
 * React 19 type compatibility overrides.
 *
 * react-markdown v10, sonner v2, and cmdk export components whose inferred
 * return types don't satisfy React 19's stricter JSX element type. These
 * overrides relax the component types so they can be used in JSX without
 * error.
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

declare module "cmdk" {
  import type { ComponentType, ReactNode } from "react";

  export const Command: ComponentType<Record<string, unknown>> & {
    Input: ComponentType<Record<string, unknown>>;
    List: ComponentType<Record<string, unknown>>;
    Empty: ComponentType<Record<string, unknown>>;
    Group: ComponentType<Record<string, unknown>>;
    Item: ComponentType<Record<string, unknown>>;
    Loading: ComponentType<Record<string, unknown>>;
    Separator: ComponentType<Record<string, unknown>>;
    Dialog: ComponentType<Record<string, unknown>>;
  };

  export function CommandDialog(props: {
    open?: boolean;
    onOpenChange?: (open: boolean) => void;
    children?: ReactNode;
    label?: string;
    overlayClassName?: string;
    className?: string;
  }): ReactNode;
}
