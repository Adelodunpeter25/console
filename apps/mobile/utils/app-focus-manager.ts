import { AppState, type AppStateStatus, Platform } from "react-native";
import { focusManager } from "@tanstack/react-query";

/**
 * Bridges React Native's AppState events with TanStack Query's focusManager.
 * When the app transitions to "active" (screen unlocked / app foregrounded),
 * TanStack Query marks queries as focused and automatically refetches stale queries.
 *
 * When backgrounded, focusManager pauses interval timers, consuming 0 background CPU/battery.
 */
export function setupAppFocusManager(): () => void {
  if (Platform.OS === "web") return () => {};

  focusManager.setEventListener((handleFocus) => {
    const subscription = AppState.addEventListener(
      "change",
      (status: AppStateStatus) => {
        handleFocus(status === "active");
      },
    );

    return () => {
      subscription.remove();
    };
  });

  return () => {
    focusManager.setEventListener(() => () => {});
  };
}
