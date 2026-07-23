import React from "react";
import { Text, View, TouchableOpacity, Alert, Platform } from "react-native";

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
  const isIos = Platform.OS === "ios";
  return (
    <View className={`h-15 flex-row border-t border-white/10 bg-[#080809] ${isIos ? "pb-2.5" : ""}`}>
      <TouchableOpacity
        className={`flex-1 items-center justify-center ${activeTab === "home" ? "bg-white/5" : ""}`}
        onPress={() => setActiveTab("home")}
      >
        <Text className={`text-xs font-medium ${activeTab === "home" ? "text-sky-400" : "text-[#9095a0]"}`}>
          Home
        </Text>
      </TouchableOpacity>
      <TouchableOpacity
        className={`flex-1 items-center justify-center ${activeTab === "chat" ? "bg-white/5" : ""}`}
        onPress={() => {
          if (!selectedSessionId) {
            Alert.alert("No Session", "Select or create a chat session on the Home tab first.");
            return;
          }
          setActiveTab("chat");
        }}
      >
        <Text className={`text-xs font-medium ${activeTab === "chat" ? "text-sky-400" : "text-[#9095a0]"}`}>
          Chat
        </Text>
      </TouchableOpacity>
      <TouchableOpacity className="flex-1 items-center justify-center" onPress={onOpenConfig}>
        <Text className="text-[#9095a0] text-xs font-medium">Config</Text>
      </TouchableOpacity>
    </View>
  );
}
