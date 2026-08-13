import React from "react";

interface DragDropZoneProps {
  children: React.ReactNode;
  onDropFiles?: (files: File[]) => void | Promise<void>;
  onDropPaths?: (paths: string[]) => void | Promise<void>;
  accept?: (file: File) => boolean;
  className?: string;
}

/** Reusable drop target with a subtle warm brown highlight matching the central theme. */
export function DragDropZone({
  children,
  onDropFiles,
  onDropPaths,
  accept,
  className = "",
}: DragDropZoneProps) {
  const [isDragging, setIsDragging] = React.useState(false);
  const dragDepth = React.useRef(0);
  const zoneRef = React.useRef<HTMLDivElement>(null);

  const handleDragEnter = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    dragDepth.current += 1;
    setIsDragging(true);
  };

  const handleDragLeave = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    dragDepth.current -= 1;
    if (dragDepth.current <= 0) {
      dragDepth.current = 0;
      setIsDragging(false);
    }
  };

  const handleDrop = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    dragDepth.current = 0;
    setIsDragging(false);

    const files = Array.from(event.dataTransfer.files).filter((file) => !accept || accept(file));
    if (files.length > 0) {
      void onDropFiles?.(files);
    }

    // In Electron, File objects have a .path property with the real OS file path
    const paths = Array.from(event.dataTransfer.files)
      .map((f) => (f as any).path)
      .filter((p): p is string => typeof p === "string" && p.length > 0);

    if (paths.length > 0) {
      void onDropPaths?.(paths);
    }
  };

  return (
    <div
      ref={zoneRef}
      className={`${className} transition-shadow ${
        isDragging
          ? "ring-2 ring-dropzone-border ring-offset-2 ring-offset-screen bg-dropzone-bg/50"
          : ""
      }`}
      onDragEnter={handleDragEnter}
      onDragOver={(event) => {
        event.preventDefault();
        event.dataTransfer.dropEffect = "copy";
      }}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {children}
    </div>
  );
}
