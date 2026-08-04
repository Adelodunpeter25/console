import React from "react";

interface ResizablePanelProps {
  /** Controlled width (px). */
  width: number;
  onWidthChange: (width: number) => void;
  /** Drag-snap limits (px). */
  minWidth?: number;
  maxWidth?: number;
  /** Called once when a drag ends — persist here. */
  onResizeEnd?: (width: number) => void;
  /** Side the handle sits on: "right" puts the handle after the panel. */
  handleSide?: "left" | "right";
  className?: string;
  children: React.ReactNode;
}

/**
 * Custom resizable panel: a drag handle on one edge updates a controlled
 * width within [minWidth, maxWidth]. The handle is a thin invisible strip so
 * it doesn't steal clicks from the content.
 */
export function ResizablePanel({
  width,
  onWidthChange,
  minWidth = 200,
  maxWidth = 600,
  onResizeEnd,
  handleSide = "right",
  className = "",
  children,
}: ResizablePanelProps) {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const draggingRef = React.useRef(false);

  const clamp = (v: number) => Math.min(Math.max(v, minWidth), maxWidth);

  const onPointerDown = (e: React.PointerEvent) => {
    e.preventDefault();
    draggingRef.current = true;
    const startX = e.clientX;
    const startWidth = width;
    const handle = e.currentTarget;
    handle.setPointerCapture(e.pointerId);

    const onMove = (ev: PointerEvent) => {
      if (!draggingRef.current) return;
      const dx = ev.clientX - startX;
      // If the handle is on the panel's right edge, dragging right grows it.
      const delta = handleSide === "right" ? dx : -dx;
      onWidthChange(clamp(startWidth + delta));
    };

    const onUp = () => {
      draggingRef.current = false;
      document.removeEventListener("pointermove", onMove);
      document.removeEventListener("pointerup", onUp);
      onResizeEnd?.(width);
    };

    document.addEventListener("pointermove", onMove);
    document.addEventListener("pointerup", onUp);
  };

  return (
    <div ref={containerRef} className={`flex h-full ${className}`}>
      {handleSide === "left" && (
        <div
          className="w-1.5 shrink-0 cursor-col-resize hover:bg-border/60 active:bg-border/80 transition-colors"
          onPointerDown={onPointerDown}
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize panel"
        />
      )}
      <div className="shrink-0 h-full overflow-hidden" style={{ width }}>
        {children}
      </div>
      {handleSide === "right" && (
        <div
          className="w-1.5 shrink-0 cursor-col-resize hover:bg-border/60 active:bg-border/80 transition-colors"
          onPointerDown={onPointerDown}
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize panel"
        />
      )}
    </div>
  );
}
