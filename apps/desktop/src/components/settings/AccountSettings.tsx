import React from "react";
import { CheckCircle2, Circle, Loader2, LogIn, RefreshCw, Save } from "lucide-react";
import { useAuthStore } from "../../store/useAuthStore";
import type { ProviderId } from "../../store/useAuthStore";
import { GlassSurface } from "../common/GlassSurface";

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
  const { status, loading, loggingIn, error, projectIds, savingProjectId, loadStatus, loginWithBrowser, loginCodebuff, saveProjectId } =
    useAuthStore();

  const [geminiProjectId, setGeminiProjectId] = React.useState("");

  React.useEffect(() => {
    loadStatus();
  }, [loadStatus]);

  React.useEffect(() => {
    setGeminiProjectId(projectIds.gemini ?? "");
  }, [projectIds.gemini]);

  const handleLogin = async (provider: ProviderId) => {
    try {
      if (provider === "codebuff") {
        await loginCodebuff();
      } else {
        await loginWithBrowser(provider);
      }
    } catch (err) {
      console.error(`Failed to login ${provider}:`, err);
    }
  };

  const providers: { id: ProviderId; label: string }[] = [
    { id: "codebuff", label: "Codebuff (Free)" },
    { id: "antigravity", label: "Antigravity" },
    { id: "gemini", label: "Gemini" },
  ];

  return (
    <div className="flex-1 overflow-y-auto px-5 py-5 bg-screen">
      <div className="mb-5">
        <h2 className="text-base font-semibold text-foreground tracking-tight">Account</h2>
        <p className="text-xs text-foreground-secondary mt-0.5">
          Manage your provider authentication and login status
        </p>
      </div>

      {loading && !status ? (
        <div className="flex items-center gap-2 text-foreground-muted">
          <Loader2 size={14} className="animate-spin" />
          <span className="text-xs">Loading auth status...</span>
        </div>
      ) : (
        <GlassSurface className="mb-4 p-4">
          <p className="text-sm font-semibold text-foreground mb-3">Provider Authentication</p>

          <div className="space-y-1">
            {providers.map(({ id, label }) => {
              const providerStatus = status
                ? (status as unknown as Record<string, { loggedIn?: boolean; email?: string } | undefined>)[id]
                : undefined;
              const loggedIn = providerStatus?.loggedIn;
              const email = providerStatus?.email;
              const isLoggingIn = loggingIn === id;

              return (
                <div key={id} className="py-2 border-b border-border last:border-b-0">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 flex-1 min-w-0">
                      {loggedIn ? (
                        <CheckCircle2 size={14} className="text-success shrink-0" />
                      ) : (
                        <Circle size={14} className="text-foreground-muted shrink-0" />
                      )}
                      <div className="min-w-0">
                        <p className="text-[13px] font-medium text-foreground leading-tight">{label}</p>
                        <p className="text-[11px] text-foreground-secondary truncate leading-tight mt-0.5">
                          {loggedIn ? email ?? "Logged in" : "Not connected"}
                        </p>
                      </div>
                    </div>

                    <button
                      onClick={() => handleLogin(id)}
                      disabled={isLoggingIn}
                      className="flex items-center gap-1 px-2.5 py-1 rounded-md text-[11px] font-normal border border-border text-foreground hover:bg-white/10 transition-colors shrink-0 disabled:opacity-50"
                    >
                      {isLoggingIn ? (
                        <Loader2 size={11} className="animate-spin" />
                      ) : loggedIn ? (
                        <RefreshCw size={11} />
                      ) : (
                        <LogIn size={11} />
                      )}
                      {loggedIn ? "Re-login" : "Login"}
                    </button>
                  </div>

                  {id === "gemini" && (
                    <div className="mt-2 pl-5">
                      <label className="block text-[11px] font-medium text-foreground-secondary mb-0.5">
                        Google Cloud project ID
                      </label>
                      <p className="text-[11px] text-foreground-muted mb-2">
                        Used for Gemini authentication when automatic discovery cannot select a
                        project. Save before logging in if your account requires it.
                      </p>
                      <div className="flex items-center gap-2">
                        <input
                          type="text"
                          value={geminiProjectId}
                          onChange={(e) => setGeminiProjectId(e.target.value)}
                          placeholder="my-project-id"
                          className="flex-1 px-2.5 py-1 rounded-md text-xs bg-black/20 border border-border text-foreground placeholder:text-foreground-muted focus:outline-none focus:border-primary/50 h-7"
                        />
                        <button
                          onClick={() => {
                            saveProjectId("gemini", geminiProjectId || undefined).catch(() => {});
                          }}
                          disabled={savingProjectId}
                          className="flex items-center gap-1 px-2.5 py-1 rounded-md text-[11px] font-normal border border-border text-foreground hover:bg-white/10 transition-colors shrink-0 disabled:opacity-50 h-7"
                        >
                          {savingProjectId ? (
                            <Loader2 size={11} className="animate-spin" />
                          ) : (
                            <Save size={11} />
                          )}
                          Save
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </GlassSurface>
      )}

      {error && (
        <div className="mt-4 rounded-xl bg-danger-muted border border-danger/30 px-4 py-3 max-w-full overflow-hidden">
          <p className="text-sm text-danger break-all whitespace-pre-wrap">{error}</p>
        </div>
      )}
    </div>
  );
}
