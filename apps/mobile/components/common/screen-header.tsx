import React from "react";
import { Text, View, TouchableOpacity, StyleSheet } from "react-native";
import { ChevronLeft, Settings, Menu } from "lucide-react-native";
import { theme } from "../../styles/theme";

interface ScreenHeaderProps {
  title: string;
  subtitle?: string;
  onBack?: () => void;
  showSettings?: boolean;
  onSettingsPress?: () => void;
  showFilter?: boolean;
  onFilterPress?: () => void;
}

export function ScreenHeader({
  title,
  subtitle,
  onBack,
  showSettings,
  onSettingsPress,
  showFilter,
  onFilterPress,
}: ScreenHeaderProps) {
  return (
    <View style={styles.headerContainer}>
      <View style={styles.leftSection}>
        {onBack ? (
          <TouchableOpacity style={styles.backButton} onPress={onBack} activeOpacity={0.7}>
            <ChevronLeft size={24} color={theme.colors.text.primary} />
          </TouchableOpacity>
        ) : null}
        <View style={styles.titleWrapper}>
          <Text style={styles.titleText}>{title}</Text>
          {subtitle ? (
            <View style={styles.badge}>
              <Text style={styles.badgeText}>{subtitle.toUpperCase()}</Text>
            </View>
          ) : null}
        </View>
      </View>

      <View style={styles.rightSection}>
        {showFilter ? (
          <TouchableOpacity style={styles.iconButton} onPress={onFilterPress} activeOpacity={0.7}>
            <Menu size={20} color={theme.colors.text.primary} />
          </TouchableOpacity>
        ) : null}
        {showSettings ? (
          <TouchableOpacity style={styles.iconButton} onPress={onSettingsPress} activeOpacity={0.7}>
            <Settings size={20} color={theme.colors.text.primary} />
          </TouchableOpacity>
        ) : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  headerContainer: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingTop: 16,
    paddingBottom: 12,
  },
  leftSection: {
    flexDirection: "row",
    alignItems: "center",
  },
  backButton: {
    marginRight: 12,
  },
  titleWrapper: {
    flexDirection: "row",
    alignItems: "center",
  },
  titleText: {
    fontSize: 22,
    fontWeight: "bold",
    color: theme.colors.text.primary,
    letterSpacing: -0.5,
  },
  badge: {
    backgroundColor: "rgba(255, 255, 255, 0.08)",
    borderColor: "rgba(255, 255, 255, 0.12)",
    borderWidth: 1,
    borderRadius: theme.roundness.full,
    paddingHorizontal: 8,
    paddingVertical: 2,
    marginLeft: 10,
  },
  badgeText: {
    fontSize: 9,
    fontFamily: theme.fonts.monoBold,
    color: theme.colors.text.secondary,
    letterSpacing: 1.5,
  },
  rightSection: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
  iconButton: {
    width: 40,
    height: 40,
    borderRadius: theme.roundness.full,
    backgroundColor: "rgba(255, 255, 255, 0.06)",
    borderColor: "rgba(255, 255, 255, 0.1)",
    borderWidth: 1,
    alignItems: "center",
    justifyContent: "center",
  },
});
