import React from "react";

export type DropPosition = "left" | "right" | "top" | "bottom" | "center";

interface WorkspaceDropzoneProps {
  position: DropPosition | null;
}

/**
 * WorkspaceDropzone — Visual dropzone overlay showing split direction
 * (Left, Right, Top, Bottom, Center) when dragging tabs or sidebar items.
 * Uses pointer-events-none so it never steals drag events from the pane.
 *
 * Outer shell is always `inset-0` so the stacking layer covers the full pane
 * even when only a half-region is highlighted (avoids zero-height % issues).
 */
export function WorkspaceDropzone({ position }: WorkspaceDropzoneProps) {
  if (!position) return null;

  const regionClasses: Record<DropPosition, string> = {
    left: "absolute top-0 bottom-0 left-0 w-1/2 bg-amber-500/30 border-l-4 border-l-amber-500",
    right: "absolute top-0 bottom-0 right-0 w-1/2 bg-amber-500/30 border-r-4 border-r-amber-500",
    top: "absolute top-0 left-0 right-0 h-1/2 bg-amber-500/30 border-t-4 border-t-amber-500",
    bottom: "absolute bottom-0 left-0 right-0 h-1/2 bg-amber-500/30 border-b-4 border-b-amber-500",
    center: "absolute inset-0 bg-amber-500/20 border-2 border-dashed border-amber-500",
  };

  return (
    <div
      aria-hidden
      className="absolute inset-0 z-[100] pointer-events-none overflow-hidden"
    >
      <div className={`${regionClasses[position]} transition-all duration-75`} />
      <div className="absolute inset-0 flex items-center justify-center">
        <span className="px-2.5 py-1 bg-black/90 text-amber-400 text-[11px] font-mono rounded border border-amber-500/50 uppercase tracking-wide shadow-lg">
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
