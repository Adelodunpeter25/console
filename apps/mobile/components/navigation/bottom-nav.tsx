import React from "react";
import { Text, View, TouchableOpacity, Alert } from "react-native";
import { styles } from "../../styles/styles";

interface BottomNavProps {
  activeTab: "home" | "chat";
  setActiveTab: (tab: "home" | "chat") => void;
  selectedSessionId: string | null;
  onOpenConfig: () => void;
}

export function BottomNav({
  activeTab,
  setActiveTab,
  selectedSessionId,
  onOpenConfig,
}: BottomNavProps) {
  return (
    <View style={styles.bottomBar}>
      <TouchableOpacity
        style={[styles.tabButton, activeTab === "home" && styles.tabButtonActive]}
        onPress={() => setActiveTab("home")}
      >
        <Text style={[styles.tabText, activeTab === "home" && styles.tabTextActive]}>
          Home
        </Text>
      </TouchableOpacity>
      <TouchableOpacity
        style={[styles.tabButton, activeTab === "chat" && styles.tabButtonActive]}
        onPress={() => {
          if (!selectedSessionId) {
            Alert.alert("No Session", "Select or create a chat session on the Home tab first.");
            return;
          }
          setActiveTab("chat");
        }}
      >
        <Text style={[styles.tabText, activeTab === "chat" && styles.tabTextActive]}>
          Chat
        </Text>
      </TouchableOpacity>
      <TouchableOpacity style={styles.tabButton} onPress={onOpenConfig}>
        <Text style={styles.tabTextSettings}>Config</Text>
      </TouchableOpacity>
    </View>
  );
}
