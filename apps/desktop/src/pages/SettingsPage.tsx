import React from "react";
import { useNavigate } from "@tanstack/react-router";
import { TitleBar } from "../components/TitleBar";
import { SettingsSidebar } from "../components/settings/SettingsSidebar";
import { ConnectionSettings } from "../components/settings/ConnectionSettings";
import { AccountSettings } from "../components/settings/AccountSettings";
import { DeletedChatsSettings } from "../components/settings/DeletedChatsSettings";
import { ResizablePanel } from "../components/common/ResizablePanel";
import { useAppStore } from "../store/useAppStore";
import type { SettingsSection } from "../components/settings/SettingsSidebar";

const SIDEBAR_MIN = 200;
const SIDEBAR_MAX = 480;

/**
 * Settings page — separate full-page route with its own sidebar.
 *
 * Layout: custom titlebar (with back button) | settings sidebar | content.
 * The settings sidebar lists available settings sections and includes a
 * "Back to App" button that navigates to the chat page. It shares the same
 * persisted width as the chat sidebar.
 */
export function SettingsPage() {
  const navigate = useNavigate();
  const [activeSection, setActiveSection] = React.useState<SettingsSection>("account");
  const sidebarWidth = useAppStore((state) => state.sidebarWidth);
  const setSidebarWidth = useAppStore((state) => state.setSidebarWidth);

  return (
    <div className="flex flex-col h-screen w-screen bg-screen overflow-hidden">
      <TitleBar title="Settings" />
      <div className="flex flex-1 overflow-hidden">
        <ResizablePanel
          width={sidebarWidth}
          onWidthChange={setSidebarWidth}
          minWidth={SIDEBAR_MIN}
          maxWidth={SIDEBAR_MAX}
          handleSide="right"
        >
          <SettingsSidebar
            width={sidebarWidth}
            active={activeSection}
            onSelect={setActiveSection}
            onBack={() => navigate({ to: "/" })}
          />
        </ResizablePanel>
        <div className="flex-1 flex flex-col h-full overflow-hidden">
          {activeSection === "account" && <AccountSettings />}
          {activeSection === "connection" && <ConnectionSettings />}
          {activeSection === "deleted-chats" && <DeletedChatsSettings />}
        </div>
      </div>
    </div>
  );
}
