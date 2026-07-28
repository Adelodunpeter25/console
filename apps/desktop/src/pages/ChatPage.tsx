import React from "react";
import { Settings as SettingsIcon } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { TitleBar } from "../components/TitleBar";
import { Sidebar } from "../components/Sidebar";
import { ChatScreen } from "../screens/ChatScreen";
import { EmptyState } from "../components/common";
import { CommandPalette } from "../components/commandpalette";
import { useAppStore, useServerStore } from "../store";

/**
 * Main chat page — the app's primary view.
 *
 * Layout: custom titlebar (with settings gear) | sidebar | chat content.
 * Command palette (⌘K / Ctrl+K) overlays the entire page.
 * When no session is selected, an empty state is shown.
 */
export function ChatPage() {
  const navigate = useNavigate();
  const { selectedSessionId } = useAppStore();
  const { init } = useServerStore();
  const [paletteOpen, setPaletteOpen] = React.useState(false);

  React.useEffect(() => {
    init();
  }, [init]);

  // ⌘K / Ctrl+K toggles the command palette
  React.useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

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
              description="Select or create a chat session from the sidebar, or press ⌘K for the command palette."
            />
          )}
        </div>
      </div>
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
    </div>
  );
}
