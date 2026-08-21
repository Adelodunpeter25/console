import React, { useEffect, useRef } from "react";
import { Animated, View, type ViewProps } from "react-native";

interface SkeletonProps extends ViewProps {
  className?: string;
  width?: number | string;
  height?: number | string;
  rounded?: "sm" | "md" | "lg" | "xl" | "2xl" | "full" | "none";
}

const roundedMap: Record<string, string> = {
  none: "rounded-none",
  sm: "rounded-sm",
  md: "rounded-md",
  lg: "rounded-lg",
  xl: "rounded-xl",
  "2xl": "rounded-2xl",
  full: "rounded-full",
};

export function Skeleton({
  className = "",
  width,
  height,
  rounded = "md",
  style,
  ...props
}: SkeletonProps) {
  const opacity = useRef(new Animated.Value(0.35)).current;

  useEffect(() => {
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(opacity, {
          toValue: 0.75,
          duration: 850,
          useNativeDriver: true,
        }),
        Animated.timing(opacity, {
          toValue: 0.35,
          duration: 850,
          useNativeDriver: true,
        }),
      ]),
    );

    animation.start();

    return () => animation.stop();
  }, [opacity]);

  const roundedClass = roundedMap[rounded] || "rounded-md";

  return (
    <Animated.View
      className={`bg-white/10 ${roundedClass} ${className}`}
      style={[
        {
          opacity,
          width: width as any,
          height: height as any,
        },
        style,
      ]}
      {...props}
    />
  );
}

/**
 * Skeleton loader for a single session row inside a project card
 */
export function SessionRowSkeleton({ isLast = false }: { isLast?: boolean }) {
  return (
    <View
      className={`flex-row items-center px-4 py-3.5 ${
        !isLast ? "border-b border-border/40" : ""
      }`}
    >
      <View className="flex-1 mr-2 gap-2">
        <Skeleton className="h-4 w-3/5" rounded="md" />
        <View className="flex-row items-center gap-2">
          <Skeleton className="h-3 w-1/4" rounded="sm" />
          <Skeleton className="h-3 w-1/6" rounded="sm" />
        </View>
      </View>
      <View className="items-end gap-1.5">
        <Skeleton className="h-4 w-12" rounded="full" />
        <Skeleton className="h-2.5 w-8" rounded="sm" />
      </View>
    </View>
  );
}

/**
 * Skeleton loader representing a project group of sessions
 */
export function ProjectSectionSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <View className="mb-6">
      {/* Section Header skeleton */}
      <View className="flex-row items-center justify-between mb-2 px-1">
        <View className="flex-row items-center gap-2">
          <Skeleton className="w-3.5 h-3.5" rounded="sm" />
          <Skeleton className="w-24 h-3.5" rounded="sm" />
        </View>
        <Skeleton className="w-6 h-6" rounded="md" />
      </View>

      {/* Card container skeleton */}
      <View className="bg-card border border-border rounded-2xl overflow-hidden">
        {Array.from({ length: rows }).map((_, i) => (
          <SessionRowSkeleton key={i} isLast={i === rows - 1} />
        ))}
      </View>
    </View>
  );
}

/**
 * Full session list loading state with multiple project sections
 */
export function SessionListSkeleton() {
  return (
    <View className="py-1">
      <ProjectSectionSkeleton rows={3} />
      <ProjectSectionSkeleton rows={2} />
    </View>
  );
}

/**
 * Chat screen loading skeleton showing simulated alternating message bubbles
 */
export function ChatScreenSkeleton() {
  return (
    <View className="flex-1 px-4 pt-3 gap-4">
      {/* User message skeleton */}
      <View className="items-end">
        <Skeleton className="h-12 w-3/5 rounded-[20px] rounded-br-md" />
        <Skeleton className="h-3 w-12 rounded-sm mt-1.5 mr-1" />
      </View>

      {/* Assistant message skeleton */}
      <View className="items-start gap-2 max-w-[85%]">
        <Skeleton className="h-4 w-28 rounded-md" />
        <Skeleton className="h-20 w-full rounded-2xl" />
        <Skeleton className="h-3 w-16 rounded-sm mt-0.5 ml-1" />
      </View>

      {/* Another User message skeleton */}
      <View className="items-end">
        <Skeleton className="h-10 w-2/5 rounded-[20px] rounded-br-md" />
        <Skeleton className="h-3 w-12 rounded-sm mt-1.5 mr-1" />
      </View>

      {/* Another Assistant message with tool skeleton */}
      <View className="items-start gap-2 w-full">
        <Skeleton className="h-12 w-full rounded-xl" />
        <Skeleton className="h-16 w-4/5 rounded-2xl" />
      </View>
    </View>
  );
}
