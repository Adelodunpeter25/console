import React from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";

interface DragDropZoneProps {
  children: React.ReactNode;
  onDropFiles?: (files: File[]) => void | Promise<void>;
  onDropPaths?: (paths: string[]) => void | Promise<void>;
  accept?: (file: File) => boolean;
  className?: string;
}

/** Reusable drop target with a rounded blue drag-over highlight. */
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

  const isInsideZone = React.useCallback((x: number, y: number) => {
    const rect = zoneRef.current?.getBoundingClientRect();
    if (!rect) return false;
    const scale = window.devicePixelRatio || 1;
    const logicalX = x / scale;
    const logicalY = y / scale;
    return (
      logicalX >= rect.left &&
      logicalX <= rect.right &&
      logicalY >= rect.top &&
      logicalY <= rect.bottom
    );
  }, []);

  React.useEffect(() => {
    if (!onDropPaths) return;
    let disposed = false;
    let unlisten: (() => void) | undefined;

    void getCurrentWindow()
      .onDragDropEvent(({ payload }) => {
        if (disposed) return;
        if (payload.type === "leave") {
          setIsDragging(false);
          return;
        }
        if (payload.type === "drop") {
          setIsDragging(false);
          const inside = isInsideZone(payload.position.x, payload.position.y);
          if (inside && payload.paths.length > 0) {
            void onDropPaths(payload.paths);
          }
          return;
        }
        const inside = isInsideZone(payload.position.x, payload.position.y);
        setIsDragging(inside);
      })
      .then((cleanup) => {
        if (disposed) cleanup();
        else unlisten = cleanup;
      });

    return () => {
      disposed = true;
      unlisten?.();
    };
  }, [isInsideZone, onDropPaths]);

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
    if (files.length > 0) void onDropFiles?.(files);
  };

  return (
    <div
      ref={zoneRef}
      className={`${className} transition-shadow ${
        isDragging
          ? "ring-2 ring-blue-500/80 ring-offset-2 ring-offset-screen bg-blue-500/[0.06]"
          : ""
      }`}
      onDragEnter={handleDragEnter}
      onDragOver={(event) => event.preventDefault()}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {children}
    </div>
  );
}
