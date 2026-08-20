import React from "react";
import { CheckCircle2, Circle, Loader2, LogIn, RefreshCw, Save } from "lucide-react";
import { useAuthStore } from "../../store/useAuthStore";
import { useProviderStore } from "../../store/useProviderStore";
import type { ProviderId, OAuthProviderId } from "../../store/useAuthStore";
import { GlassSurface } from "../common/GlassSurface";

/**
 * Account settings panel — shows per-provider login status and login buttons.
 *
 * Providers are rendered dynamically from the server's provider catalog
 * (`useProviderStore`), so new providers (e.g. Codebuff) appear automatically
 * without UI changes. Each provider's `authMethod` decides the login flow:
 *   - "oauth"        → full OAuth browser flow (local callback server)
 *   - "device-code"  → Codebuff-style device-code flow (open URL, poll status)
 *   - "none"         → no login needed (e.g. the free OpenCode Zen endpoint)
 */
export function AccountSettings() {
  const { status, loading, loggingIn, error, projectIds, savingProjectId, loadStatus, loginWithBrowser, loginCodebuff, saveProjectId } =
    useAuthStore();
  const {
    providers: catalogProviders,
    loadingProviders,
    loadProviders,
  } = useProviderStore();

  const [geminiProjectId, setGeminiProjectId] = React.useState("");

  React.useEffect(() => {
    loadStatus();
  }, [loadStatus]);

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    setGeminiProjectId(projectIds.gemini ?? "");
  }, [projectIds.gemini]);

  const handleLogin = async (provider: ProviderId, authMethod?: string) => {
    try {
      if (authMethod === "device-code") {
        await loginCodebuff();
      } else {
        await loginWithBrowser(provider);
      }
    } catch (err) {
      console.error(`Failed to login ${provider}:`, err);
    }
  };

  // Render providers that can authenticate (OAuth / device-code). Providers
  // with authMethod "none" have nothing to connect.
  const providers = catalogProviders.filter((p) => p.authMethod !== "none");

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

          {loadingProviders && providers.length === 0 ? (
            <div className="flex items-center gap-2 text-foreground-muted">
              <Loader2 size={14} className="animate-spin" />
              <span className="text-xs">Loading providers...</span>
            </div>
          ) : (
            <div className="space-y-1">
              {providers.map((provider) => {
                const id = provider.name as ProviderId;
                const label = provider.displayName;
                const providerStatus = status?.[id as OAuthProviderId];
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
                        onClick={() => handleLogin(id, provider.authMethod)}
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
          )}
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