import React from "react";
import { CheckCircle2, Circle, Loader2, LogIn, RefreshCw } from "lucide-react";
import { useAuthStore } from "../../store";
import type { ProviderId } from "../../store";
import { GlassSurface } from "../common";

/**
 * Account settings panel — shows per-provider OAuth login status and
 * login/re-login buttons.
 *
 * The OAuth flow is fully automatic:
 *  1. Click "Login" → Tauri starts a local callback server on the OAuth port
 *  2. Opens the Google auth URL in the system browser
 *  3. User selects their Google account
 *  4. Browser redirects to localhost:port/callback?code=...
 *  5. The local callback server catches the redirect, extracts the code
 *  6. The code is submitted to the backend for token exchange
 *  7. Status refreshes to show "Connected" with the account email
 *
 * No manual code copying — just click, authenticate, done.
 */
export function AccountSettings() {
  const { status, loading, loggingIn, error, loadStatus, loginWithBrowser } = useAuthStore();

  React.useEffect(() => {
    loadStatus();
  }, [loadStatus]);

  const handleLogin = async (provider: ProviderId) => {
    try {
      await loginWithBrowser(provider);
    } catch (err) {
      console.error(`Failed to login ${provider}:`, err);
    }
  };

  const providers: { id: ProviderId; label: string }[] = [
    { id: "antigravity", label: "Antigravity" },
    { id: "gemini", label: "Gemini" },
  ];

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6 bg-screen">
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-foreground tracking-tight">Account</h2>
        <p className="text-sm text-foreground-secondary mt-1">
          Manage your provider authentication and login status
        </p>
      </div>

      {loading && !status ? (
        <div className="flex items-center gap-2 text-foreground-muted">
          <Loader2 size={16} className="animate-spin" />
          <span className="text-sm">Loading auth status...</span>
        </div>
      ) : (
        <GlassSurface className="mb-4">
          <p className="text-base font-semibold text-foreground mb-4">Provider Authentication</p>

          <div className="space-y-1">
            {providers.map(({ id, label }) => {
              const providerStatus = status?.[id];
              const loggedIn = providerStatus?.loggedIn;
              const email = providerStatus?.email;
              const isLoggingIn = loggingIn === id;

              return (
                <div
                  key={id}
                  className="flex items-center justify-between py-3 border-b border-border last:border-b-0"
                >
                  <div className="flex items-center gap-2.5 flex-1 min-w-0">
                    {loggedIn ? (
                      <CheckCircle2 size={16} className="text-success shrink-0" />
                    ) : (
                      <Circle size={16} className="text-foreground-muted shrink-0" />
                    )}
                    <div className="min-w-0">
                      <p className="text-sm font-semibold text-foreground">{label}</p>
                      <p className="text-xs text-foreground-secondary truncate">
                        {loggedIn ? email ?? "Logged in" : "Not connected"}
                      </p>
                    </div>
                  </div>

                  <button
                    onClick={() => handleLogin(id)}
                    disabled={isLoggingIn}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold border border-border text-foreground hover:bg-white/10 transition-colors shrink-0 disabled:opacity-50"
                  >
                    {isLoggingIn ? (
                      <Loader2 size={12} className="animate-spin" />
                    ) : loggedIn ? (
                      <RefreshCw size={12} />
                    ) : (
                      <LogIn size={12} />
                    )}
                    {loggedIn ? "Re-login" : "Login"}
                  </button>
                </div>
              );
            })}
          </div>
        </GlassSurface>
      )}

      {error && (
        <div className="mt-4 rounded-xl bg-danger-muted border border-danger/30 px-4 py-3">
          <p className="text-sm text-danger">{error}</p>
        </div>
      )}
    </div>
  );
}
