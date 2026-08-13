import React from "react";

export type DropPosition = "left" | "right" | "top" | "bottom" | "center";

interface WorkspaceDropzoneProps {
  position: DropPosition | null;
}

/**
 * WorkspaceDropzone — Visual dropzone overlay showing split direction
 * (Left, Right, Top, Bottom, Center) when dragging tabs or sidebar items.
 * Uses centralized warm-brown / amber theme tokens matching user message cards.
 */
export function WorkspaceDropzone({ position }: WorkspaceDropzoneProps) {
  if (!position) return null;

  const regionClasses: Record<DropPosition, string> = {
    left: "absolute top-0 bottom-0 left-0 w-1/2 bg-dropzone-bg border-l-2 border-l-dropzone-border backdrop-blur-[1px]",
    right: "absolute top-0 bottom-0 right-0 w-1/2 bg-dropzone-bg border-r-2 border-r-dropzone-border backdrop-blur-[1px]",
    top: "absolute top-0 left-0 right-0 h-1/2 bg-dropzone-bg border-t-2 border-t-dropzone-border backdrop-blur-[1px]",
    bottom: "absolute bottom-0 left-0 right-0 h-1/2 bg-dropzone-bg border-b-2 border-b-dropzone-border backdrop-blur-[1px]",
    center: "absolute inset-0 bg-dropzone-bg border border-dashed border-dropzone-border backdrop-blur-[1px]",
  };

  return (
    <div
      aria-hidden
      className="absolute inset-0 z-[100] pointer-events-none overflow-hidden"
    >
      <div className={`${regionClasses[position]} transition-all duration-75`} />
      <div className="absolute inset-0 flex items-center justify-center">
        <span className="px-2.5 py-1 bg-dropzone-badge-bg text-dropzone-badge-text text-[11px] font-mono rounded border border-dropzone-badge-border uppercase tracking-wide shadow-lg">
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

  if (x < 0.18) return "left";
  if (x > 0.82) return "right";
  if (y < 0.18) return "top";
  if (y > 0.90) return "bottom";
  return "center";
}
