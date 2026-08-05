import React from "react";
import { Wifi, WifiOff } from "lucide-react";
import { useServerStore } from "../../store";
import { GlassSurface } from "../common";

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
    <div className="flex-1 overflow-y-auto px-6 py-6 bg-screen">
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-foreground tracking-tight">Connection</h2>
        <p className="text-sm text-foreground-secondary mt-1">
          Configure your Console backend server connection
        </p>
      </div>

      {/* Connection Endpoint Card */}
      <GlassSurface className="mb-4">
        <div className="flex items-center justify-between mb-3">
          <span className="text-base font-semibold text-foreground">Backend Server Endpoint</span>
          <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-white/10 border border-border">
            {connected ? (
              <Wifi size={12} className="text-foreground" />
            ) : (
              <WifiOff size={12} className="text-foreground-secondary" />
            )}
            <span className="text-xs font-bold text-foreground">
              {connected ? "Connected" : "Disconnected"}
            </span>
          </div>
        </div>

        <p className="text-sm text-foreground-secondary mb-4">
          HTTP URL of your running Console backend server instance:
        </p>

        <input
          type="text"
          value={inputUrl}
          onChange={(e) => setInputUrl(e.target.value)}
          placeholder="http://localhost:3000"
          className="w-full h-12 bg-card-alt border border-border rounded-xl px-4 text-foreground text-sm font-mono mb-4 outline-none focus:border-white/30 transition-colors"
        />

        <div className="flex gap-3 justify-end">
          <button
            onClick={testConnection}
            className="px-4 py-2.5 rounded-full bg-transparent border border-border text-sm font-semibold text-foreground hover:bg-white/10 transition-colors"
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
            className="px-5 py-2.5 rounded-full bg-white text-sm font-bold text-black hover:bg-white/90 transition-colors disabled:opacity-50"
          >
            {saving ? "Saving..." : "Save Changes"}
          </button>
        </div>
      </GlassSurface>

      {/* App Environment Info Card */}
      <GlassSurface className="mb-4">
        <p className="text-base font-semibold text-foreground mb-3">App Info & Diagnostics</p>

        <InfoRow label="Console Desktop Version" value="0.1.0" />
        <InfoRow label="Framework" value="Tauri v2" />
        <InfoRow label="Frontend" value="React 19 + Vite 6" />
        <InfoRow label="Styling" value="Tailwind CSS v4" />
        <InfoRow label="State Management" value="Zustand v5" />
        <InfoRow label="Routing" value="TanStack Router" last />
      </GlassSurface>

      {/* About Card */}
      <GlassSurface className="mb-8">
        <p className="text-base font-semibold text-foreground mb-2">About Console Desktop</p>
        <p className="text-sm text-foreground-secondary leading-6">
          Console Desktop is a Tauri v2 application that connects to the Console agent server. It
          provides a native desktop experience for managing projects, chat sessions, and AI-powered
          coding agents. The Rust backend handles all network communication and filesystem
          operations, while the React frontend delivers a fast, responsive UI.
        </p>
      </GlassSurface>
    </div>
  );
}

function InfoRow({ label, value, last }: { label: string; value: string; last?: boolean }) {
  return (
    <div className={`flex justify-between py-2.5 ${last ? "" : "border-b border-border"}`}>
      <span className="text-sm text-foreground-secondary">{label}</span>
      <span className="text-sm font-mono text-foreground">{value}</span>
    </div>
  );
}
