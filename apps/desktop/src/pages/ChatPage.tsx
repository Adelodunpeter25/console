import React from "react";
import { getCurrentWindow, PhysicalSize } from "@tauri-apps/api/window";
import { TitleBar } from "../components/TitleBar";
import { Sidebar } from "../components/sidebar/Sidebar";
import { WorkspaceLayout } from "../layout";
import { ResizablePanel } from "../components/common/ResizablePanel";
import { CommandPalette } from "../components/commandpalette/CommandPalette";
import { useAppStore } from "../store/useAppStore";
import { useServerStore } from "../store/useServerStore";
import { getSidebarWidth, setSidebarWidth, getWindowSize, setWindowSize } from "../lib/ui-store";

const SIDEBAR_MIN = 200;
const SIDEBAR_MAX = 480;
const SIDEBAR_DEFAULT = 288;

/**
 * Main chat page — the app's primary view.
 *
 * Layout: custom titlebar (with sidebar toggle) | sidebar | chat content.
 * Command palette (⌘K / Ctrl+K) overlays the entire page.
 * When no session is selected, an empty state is shown.
 */
export function ChatPage() {
  const sidebarOpen = useAppStore((state) => state.sidebarOpen);
  const { init } = useServerStore();
  const [paletteOpen, setPaletteOpen] = React.useState(false);
  const [sidebarWidth, setSidebarWidthState] = React.useState(SIDEBAR_DEFAULT);

  // Restore persisted sidebar width + window size on mount.
  React.useEffect(() => {
    init();
    getSidebarWidth().then((w) => {
      if (w != null) setSidebarWidthState(Math.min(Math.max(w, SIDEBAR_MIN), SIDEBAR_MAX));
    });
    const restoreWindow = async () => {
      const size = await getWindowSize();
      if (size) {
        try {
          await getCurrentWindow().setSize(new PhysicalSize(size.width, size.height));
        } catch {
          // Ignore — window may not be resizable in some platforms.
        }
      }
    };
    restoreWindow();
  }, [init]);

  // Persist window size on resize (debounced).
  React.useEffect(() => {
    const win = getCurrentWindow();
    let timer: ReturnType<typeof setTimeout> | null = null;
    const onResize = async () => {
      const size = await win.innerSize();
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        setWindowSize({ width: size.width, height: size.height }).catch(() => {});
      }, 400);
    };
    const unlisten = win.onResized(onResize);
    return () => {
      unlisten.then((fn) => fn());
      if (timer) clearTimeout(timer);
    };
  }, []);

  const handleSidebarResizeEnd = (width: number) => {
    setSidebarWidth(width).catch(() => {});
  };

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
      <TitleBar />
      <div className="flex flex-1 overflow-hidden">
        {sidebarOpen && (
          <ResizablePanel
            width={sidebarWidth}
            onWidthChange={setSidebarWidthState}
            minWidth={SIDEBAR_MIN}
            maxWidth={SIDEBAR_MAX}
            onResizeEnd={handleSidebarResizeEnd}
            handleSide="right"
          >
            <Sidebar width={sidebarWidth} />
          </ResizablePanel>
        )}
        <div className="flex-1 flex flex-col h-full overflow-hidden">
          <WorkspaceLayout />
        </div>
      </div>
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
    </div>
  );
}
