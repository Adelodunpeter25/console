import React from "react";
import { useNavigate } from "@tanstack/react-router";
import { TitleBar } from "../components/TitleBar";
import { SettingsSidebar, ConnectionSettings } from "../components/settings";
import { ResizablePanel } from "../components/common";
import { getSidebarWidth, setSidebarWidth } from "../lib/ui-store";
import type { SettingsSection } from "../components/settings";

const SIDEBAR_MIN = 200;
const SIDEBAR_MAX = 480;
const SIDEBAR_DEFAULT = 288;

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
  const [activeSection, setActiveSection] = React.useState<SettingsSection>("connection");
  const [sidebarWidth, setSidebarWidthState] = React.useState(SIDEBAR_DEFAULT);

  // Restore persisted sidebar width on mount.
  React.useEffect(() => {
    getSidebarWidth().then((w) => {
      if (w != null) setSidebarWidthState(Math.min(Math.max(w, SIDEBAR_MIN), SIDEBAR_MAX));
    });
  }, []);

  const handleSidebarResizeEnd = (width: number) => {
    setSidebarWidth(width).catch(() => {});
  };

  return (
    <div className="flex flex-col h-screen w-screen bg-screen overflow-hidden">
      <TitleBar title="Settings" />
      <div className="flex flex-1 overflow-hidden">
        <ResizablePanel
          width={sidebarWidth}
          onWidthChange={setSidebarWidthState}
          minWidth={SIDEBAR_MIN}
          maxWidth={SIDEBAR_MAX}
          onResizeEnd={handleSidebarResizeEnd}
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
          {activeSection === "connection" && <ConnectionSettings />}
        </div>
      </div>
    </div>
  );
}
