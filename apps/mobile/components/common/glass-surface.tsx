import React from "react";
import { View, ViewProps } from "react-native";

interface GlassSurfaceProps extends ViewProps {
  children: React.ReactNode;
  className?: string;
}

export function GlassSurface({ children, className = "", style, ...props }: GlassSurfaceProps) {
  return (
    <View
      className={`bg-[#121316]/90 border border-white/10 rounded-2xl p-4 shadow-2xl ${className}`}
      style={style}
      {...props}
    >
      {children}
    </View>
  );
}
