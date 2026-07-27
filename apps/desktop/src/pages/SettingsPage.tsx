import React from "react";
import { useNavigate } from "@tanstack/react-router";
import { TitleBar } from "../components/TitleBar";
import { SettingsSidebar, ConnectionSettings } from "../components/settings";
import type { SettingsSection } from "../components/settings";

/**
 * Settings page — separate full-page route with its own sidebar.
 *
 * Layout: custom titlebar (with back button) | settings sidebar | content.
 * The settings sidebar lists available settings sections and includes a
 * "Back to App" button that navigates to the chat page.
 */
export function SettingsPage() {
  const navigate = useNavigate();
  const [activeSection, setActiveSection] = React.useState<SettingsSection>("connection");

  return (
    <div className="flex flex-col h-screen w-screen bg-screen overflow-hidden">
      <TitleBar title="Settings" />
      <div className="flex flex-1 overflow-hidden">
        <SettingsSidebar
          active={activeSection}
          onSelect={setActiveSection}
          onBack={() => navigate({ to: "/" })}
        />
        <div className="flex-1 flex flex-col h-full overflow-hidden">
          {activeSection === "connection" && <ConnectionSettings />}
        </div>
      </div>
    </div>
  );
}
