import React from "react";
import { View, Text } from "react-native";

interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}

export function EmptyState({
  icon,
  title,
  description,
  action,
  className = "",
}: EmptyStateProps) {
  return (
    <View className={`items-center justify-center py-16 px-6 ${className}`}>
      {icon ? (
        <View className="mb-3.5 items-center justify-center opacity-70">
          {icon}
        </View>
      ) : null}
      <Text className="text-base font-semibold text-foreground text-center tracking-tight">
        {title}
      </Text>
      {description ? (
        <Text className="text-xs text-foreground-secondary text-center mt-1.5 leading-5 max-w-[280px]">
          {description}
        </Text>
      ) : null}
      {action ? <View className="mt-5">{action}</View> : null}
    </View>
  );
}
