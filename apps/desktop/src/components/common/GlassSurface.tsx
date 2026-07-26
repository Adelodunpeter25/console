import React from "react";

interface GlassSurfaceProps {
  children?: React.ReactNode;
  className?: string;
  flat?: boolean;
}

export function GlassSurface({ children, className = "", flat = false }: GlassSurfaceProps) {
  return (
    <div className={`${flat ? "glass-surface-flat" : "glass-surface"} ${className}`}>
      {children}
    </div>
  );
}
