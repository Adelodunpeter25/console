import React from "react";
import { SplitDirection } from "./types";

export type DropPosition = "left" | "right" | "top" | "bottom" | "center";

interface WorkspaceDropzoneProps {
  position: DropPosition | null;
}

/**
 * WorkspaceDropzone — Visual dropzone overlay showing split direction
 * (Left, Right, Top, Bottom, Center) when dragging tabs or sidebar items.
 */
export function WorkspaceDropzone({ position }: WorkspaceDropzoneProps) {
  if (!position) return null;

  const overlayClasses: Record<DropPosition, string> = {
    left: "top-0 left-0 w-1/2 h-full bg-amber-500/20 border-l-4 border-l-amber-500",
    right: "top-0 right-0 w-1/2 h-full bg-amber-500/20 border-r-4 border-r-amber-500",
    top: "top-0 left-0 w-full h-1/2 bg-amber-500/20 border-t-4 border-t-amber-500",
    bottom: "bottom-0 left-0 w-full h-1/2 bg-amber-500/20 border-b-4 border-b-amber-500",
    center: "inset-0 bg-amber-500/10 border-2 border-dashed border-amber-500/60",
  };

  return (
    <div
      className={`absolute z-50 pointer-events-none transition-all duration-75 ${overlayClasses[position]}`}
    >
      <div className="absolute inset-0 flex items-center justify-center">
        <span className="px-2 py-1 bg-black/80 text-amber-400 text-[11px] font-mono rounded border border-amber-500/40 uppercase tracking-wide">
          Drop {position}
        </span>
      </div>
    </div>
  );
}

export function calcDropPosition(
  rect: DOMRect,
  clientX: number,
  clientY: number,
): DropPosition {
  const x = (clientX - rect.left) / rect.width;
  const y = (clientY - rect.top) / rect.height;

  if (x < 0.25) return "left";
  if (x > 0.75) return "right";
  if (y < 0.25) return "top";
  if (y > 0.75) return "bottom";
  return "center";
}
