import React from "react";

export type DropPosition = "left" | "right" | "center";

interface WorkspaceDropzoneProps {
  position: DropPosition | null;
}

/**
 * WorkspaceDropzone — Visual dropzone overlay for side-by-side (Left, Right, Center) tab docking.
 * Top and bottom vertical splitting is omitted.
 */
export function WorkspaceDropzone({ position }: WorkspaceDropzoneProps) {
  if (!position) return null;

  const regionClasses: Record<DropPosition, string> = {
    left: "absolute top-0 bottom-0 left-0 w-1/2 bg-dropzone-bg border-l-2 border-l-dropzone-border backdrop-blur-[1px]",
    right: "absolute top-0 bottom-0 right-0 w-1/2 bg-dropzone-bg border-r-2 border-r-dropzone-border backdrop-blur-[1px]",
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
): DropPosition {
  const x = (clientX - rect.left) / rect.width;

  if (x < 0.22) return "left";
  if (x > 0.78) return "right";
  return "center";
}
