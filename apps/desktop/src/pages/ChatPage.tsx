import React from "react";
import { Settings as SettingsIcon } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { TitleBar } from "../components/TitleBar";
import { Sidebar } from "../components/Sidebar";
import { ChatScreen } from "../screens/ChatScreen";
import { EmptyState } from "../components/common";
import { useAppStore, useServerStore } from "../store";

/**
 * Main chat page — the app's primary view.
 *
 * Layout: custom titlebar (with settings gear) | sidebar | chat content.
 * When no session is selected, an empty state is shown.
 */
export function ChatPage() {
  const navigate = useNavigate();
  const { selectedSessionId } = useAppStore();
  const { init } = useServerStore();

  React.useEffect(() => {
    init();
  }, [init]);

  return (
    <div className="flex flex-col h-screen w-screen bg-screen overflow-hidden">
      <TitleBar
        rightAction={{
          icon: <SettingsIcon size={16} />,
          label: "Settings",
          onClick: () => navigate({ to: "/settings" }),
        }}
      />
      <div className="flex flex-1 overflow-hidden">
        <Sidebar />
        <div className="flex-1 flex flex-col h-full overflow-hidden">
          {selectedSessionId ? (
            <ChatScreen />
          ) : (
            <EmptyState
              title="No Session Selected"
              description="Select or create a chat session from the sidebar to start chatting with the agent."
            />
          )}
        </div>
      </div>
    </div>
  );
}
