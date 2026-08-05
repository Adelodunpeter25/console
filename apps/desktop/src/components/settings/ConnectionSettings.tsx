import React from "react";
import { Wifi, WifiOff } from "lucide-react";
import { useServerStore } from "../../store/useServerStore";
import { GlassSurface } from "../common/GlassSurface";

/**
 * Connection settings panel — backend server endpoint configuration,
 * connection testing, and app diagnostics.
 *
 * Extracted from the former SettingsScreen into a dedicated settings
 * section component.
 */
export function ConnectionSettings() {
  const { backendUrl, connected, testing, init, setUrl, testConnection } = useServerStore();
  const [inputUrl, setInputUrl] = React.useState(backendUrl);
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    init();
  }, [init]);

  React.useEffect(() => {
    setInputUrl(backendUrl);
  }, [backendUrl]);

  const handleSave = async () => {
    if (!inputUrl.trim()) return;
    setSaving(true);
    try {
      await setUrl(inputUrl);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto px-5 py-5 bg-screen">
      <div className="mb-5">
        <h2 className="text-base font-semibold text-foreground tracking-tight">Connection</h2>
        <p className="text-xs text-foreground-secondary mt-0.5">
          Configure your Console backend server connection
        </p>
      </div>

      {/* Connection Endpoint Card */}
      <GlassSurface className="mb-4 p-4">
        <div className="flex items-center justify-between mb-3">
          <span className="text-sm font-semibold text-foreground">Backend Server Endpoint</span>
          <div className="flex items-center gap-1 px-2 py-0.5 rounded-md bg-white/10 border border-border">
            {connected ? (
              <Wifi size={10} className="text-foreground" />
            ) : (
              <WifiOff size={10} className="text-foreground-secondary" />
            )}
            <span className="text-[10px] font-bold text-foreground">
              {connected ? "Connected" : "Disconnected"}
            </span>
          </div>
        </div>

        <p className="text-xs text-foreground-secondary mb-3">
          HTTP URL of your running Console backend server instance:
        </p>

        <input
          type="text"
          value={inputUrl}
          onChange={(e) => setInputUrl(e.target.value)}
          placeholder="http://localhost:3000"
          className="w-full h-8 bg-card-alt border border-border rounded-md px-2.5 text-foreground text-xs font-mono mb-3 outline-none focus:border-white/30 transition-colors"
        />

        <div className="flex gap-2 justify-end">
          <button
            onClick={testConnection}
            className="px-2.5 py-1 rounded-md bg-transparent border border-border text-xs font-normal text-white hover:bg-white/10 transition-colors"
          >
            {testing === "testing"
              ? "Testing..."
              : testing === "success"
                ? "Online"
                : testing === "error"
                  ? "Offline"
                  : "Test Connection"}
          </button>

          <button
            onClick={handleSave}
            disabled={saving}
            className="px-3 py-1 rounded-md bg-white text-xs font-semibold text-black hover:bg-white/90 transition-colors disabled:opacity-50"
          >
            {saving ? "Saving..." : "Save Changes"}
          </button>
        </div>
      </GlassSurface>
    </div>
  );
}
