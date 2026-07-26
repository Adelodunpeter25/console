import React from "react";
import { Sidebar } from "./components/Sidebar";
import { HomeScreen } from "./screens/HomeScreen";
import { ChatScreen } from "./screens/ChatScreen";
import { SettingsScreen } from "./screens/SettingsScreen";
import { useAppStore, useServerStore } from "./store";

export function App() {
  const { activeView } = useAppStore();
  const { init } = useServerStore();

  React.useEffect(() => {
    init();
  }, [init]);

  return (
    <div className="flex h-screen w-screen bg-screen overflow-hidden">
      <Sidebar />
      <div className="flex-1 flex flex-col h-full overflow-hidden">
        {activeView === "home" && <HomeScreen />}
        {activeView === "chat" && <ChatScreen />}
        {activeView === "settings" && <SettingsScreen />}
      </div>
    </div>
  );
}
