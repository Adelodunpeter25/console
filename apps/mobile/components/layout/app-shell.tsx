import type { PropsWithChildren } from "react";
import { View } from "react-native";

/**
 * Root chrome for authenticated screens. Top/bottom safe areas are applied by
 * ScreenHeader and the bottom bars (SearchBar / Composer) so we don't stack
 * SafeAreaView padding on top of manual insets (which left a large gap under
 * the Android status bar). KeyboardProvider lives once in index.tsx.
 */
export function AppShell({ children }: PropsWithChildren) {
  return <View style={{ flex: 1, backgroundColor: "#0a0a0b" }}>{children}</View>;
}
