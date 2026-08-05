import React from "react";
import { CheckCircle2, Circle, Loader2, LogIn, RefreshCw, ExternalLink } from "lucide-react";
import { openUrl } from "@tauri-apps/plugin-opener";
import { useAuthStore } from "../../store";
import type { ProviderId } from "../../store";
import { GlassSurface } from "../common";

/**
 * Account settings panel — shows per-provider OAuth login status and
 * login/re-login buttons.
 *
 * The OAuth flow:
 *  1. Click "Login" → fetches the OAuth URL from the backend
 *  2. Opens the URL in the system browser
 *  3. User authenticates, browser redirects to localhost:port/callback?code=...
 *  4. User copies the code from the redirect URL and pastes it into the
 *     code input field that appears
 *  5. Click "Complete Login" → submits the code to the backend for token exchange
 *  6. Status refreshes to show "Connected" with the account email
 */
export function AccountSettings() {
  const { status, loading, pendingProvider, error, loadStatus, startLogin, completeLogin } =
    useAuthStore();
  const [codeInputs, setCodeInputs] = React.useState<Record<ProviderId, string>>({
    gemini: "",
    antigravity: "",
  });
  const [submitting, setSubmitting] = React.useState<ProviderId | null>(null);

  React.useEffect(() => {
    loadStatus();
  }, [loadStatus]);

  const handleLoginClick = async (provider: ProviderId) => {
    try {
      const result = await startLogin(provider);
      await openUrl(result.authUrl);
    } catch (err) {
      console.error(`Failed to start ${provider} login:`, err);
    }
  };

  const handleCompleteLogin = async (provider: ProviderId) => {
    const code = codeInputs[provider].trim();
    if (!code) return;
    setSubmitting(provider);
    try {
      await completeLogin(provider, code);
      setCodeInputs((prev) => ({ ...prev, [provider]: "" }));
    } catch (err) {
      console.error(`Failed to complete ${provider} login:`, err);
    } finally {
      setSubmitting(null);
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
              const isPending = pendingProvider === id;
              const isSubmitting = submitting === id;

              return (
                <div
                  key={id}
                  className="py-3 border-b border-border last:border-b-0"
                >
                  {/* Status row */}
                  <div className="flex items-center justify-between">
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

                    {/* Action button */}
                    <button
                      onClick={() => handleLoginClick(id)}
                      disabled={isPending}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold border border-border text-foreground hover:bg-white/10 transition-colors shrink-0 disabled:opacity-50"
                    >
                      {isPending ? (
                        <Loader2 size={12} className="animate-spin" />
                      ) : loggedIn ? (
                        <RefreshCw size={12} />
                      ) : (
                        <LogIn size={12} />
                      )}
                      {loggedIn ? "Re-login" : "Login"}
                    </button>
                  </div>

                  {/* Code input — visible when a login flow is pending */}
                  {isPending && (
                    <div className="mt-3 flex items-center gap-2">
                      <input
                        type="text"
                        value={codeInputs[id]}
                        onChange={(e) =>
                          setCodeInputs((prev) => ({ ...prev, [id]: e.target.value }))
                        }
                        placeholder="Paste authorization code..."
                        className="flex-1 h-9 bg-card-alt border border-border rounded-lg px-3 text-sm font-mono text-foreground outline-none focus:border-white/30 transition-colors"
                        onKeyDown={(e) => {
                          if (e.key === "Enter") handleCompleteLogin(id);
                        }}
                      />
                      <button
                        onClick={() => handleCompleteLogin(id)}
                        disabled={isSubmitting || !codeInputs[id].trim()}
                        className="px-3 py-1.5 rounded-lg bg-white text-xs font-bold text-black hover:bg-white/90 transition-colors disabled:opacity-50"
                      >
                        {isSubmitting ? (
                          <Loader2 size={12} className="animate-spin" />
                        ) : (
                          "Complete"
                        )}
                      </button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {/* Help text */}
          <div className="mt-4 pt-3 border-t border-border">
            <p className="text-xs text-foreground-muted leading-relaxed flex items-start gap-1.5">
              <ExternalLink size={12} className="shrink-0 mt-0.5" />
              <span>
                Click "Login" to open your browser and authenticate. After redirecting, copy the
                authorization code from the URL and paste it above to complete login.
              </span>
            </p>
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
