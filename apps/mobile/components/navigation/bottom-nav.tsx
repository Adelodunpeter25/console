import React from "react";
import { Text, View, TouchableOpacity, Alert, Platform } from "react-native";

interface BottomNavProps {
  activeTab: "home" | "chat" | "settings";
  setActiveTab: (tab: "home" | "chat" | "settings") => void;
  selectedSessionId: string | null;
}

export function BottomNav({ activeTab, setActiveTab, selectedSessionId }: BottomNavProps) {
  const isIos = Platform.OS === "ios";
  return (
    <View className={`h-16 flex-row border-t border-white/10 bg-[#080809] ${isIos ? "pb-3" : ""}`}>
      <TouchableOpacity
        className={`flex-1 items-center justify-center ${activeTab === "home" ? "bg-white/10" : ""}`}
        onPress={() => setActiveTab("home")}
      >
        <Text
          className={`text-sm ${activeTab === "home" ? "text-white font-bold" : "text-zinc-400 font-medium"}`}
        >
          Home
        </Text>
      </TouchableOpacity>

      <TouchableOpacity
        className={`flex-1 items-center justify-center ${activeTab === "chat" ? "bg-white/10" : ""}`}
        onPress={() => {
          if (!selectedSessionId) {
            Alert.alert("No Session", "Select or create a chat session on the Home tab first.");
            return;
          }
          setActiveTab("chat");
        }}
      >
        <Text
          className={`text-sm ${activeTab === "chat" ? "text-white font-bold" : "text-zinc-400 font-medium"}`}
        >
          Chat
        </Text>
      </TouchableOpacity>

      <TouchableOpacity
        className={`flex-1 items-center justify-center ${activeTab === "settings" ? "bg-white/10" : ""}`}
        onPress={() => setActiveTab("settings")}
      >
        <Text
          className={`text-sm ${activeTab === "settings" ? "text-white font-bold" : "text-zinc-400 font-medium"}`}
        >
          Settings
        </Text>
      </TouchableOpacity>
    </View>
  );
}
