//! Auth DTOs mirroring the server's `/api/auth/*` contracts.
//!
//! The wire types are now shared protobuf (console_proto, see
//! proto/console/v1/auth.proto) and re-exported below. `OAuthProviderId`
//! stays a hand-written UI-side enum — it is a closed set of ids the UI
//! switches on, not something the server enumerates.

use serde::{Deserialize, Serialize};

pub use console_proto::{
    AccountMutationResponse, AuthStatusResponse, GitHubAuthStatus, GitHubLogoutResponse,
    OAuthCallbackResponse, OAuthLoginUrlResponse, ProjectIdResponse, ProviderAccount,
    ProviderAuthStatus, ProviderCredentialStatus, ProviderCredentialStatuses,
    SetProviderCredentialResponse,
};

/// OAuth-capable providers with credential state.
#[derive(Clone, Copy, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub enum OAuthProviderId {
    Antigravity,
    Codex,
    Claude,
}

impl OAuthProviderId {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Antigravity => "antigravity",
            Self::Codex => "codex",
            Self::Claude => "claude",
        }
    }
}

/// The login status for one provider id, flattened out of the proto's
/// `Option<ProviderAuthStatus>`.
///
/// Provider ids are an open vocabulary on the wire (the server also reports
/// `devin`, and `openai`/`anthropic` are catalog aliases), so an unknown id
/// is "not reported" rather than logged out — hence `None`.
///
/// The `ProviderAuthStatus` sub-messages are optional in proto because the
/// wire may omit an empty one, but the server always sends every provider, so
/// the common case is `Some`.
pub fn auth_status_for<'a>(
    status: &'a AuthStatusResponse,
    provider: &str,
) -> Option<&'a ProviderAuthStatus> {
    match provider {
        "antigravity" => status.antigravity.as_ref(),
        // "openai" is the catalog's name for the codex provider.
        "codex" | "openai" => status.codex.as_ref(),
        // "anthropic" is the catalog's name for the claude provider.
        "claude" | "anthropic" => status.claude.as_ref(),
        _ => None,
    }
}

/// Applies one provider's flat `ProviderAuthStatus` — typically the body of
/// an `AccountMutationResponse` — into a full status payload, so a Use/Remove
/// click updates the provider row from the response's data without a second
/// `GET /api/auth/status` round-trip.
pub fn merge_provider_status(status: &mut AuthStatusResponse, provider: &str, patch: ProviderAuthStatus) {
    let slot = match provider {
        "antigravity" => &mut status.antigravity,
        "codex" | "openai" => &mut status.codex,
        "claude" | "anthropic" => &mut status.claude,
        _ => return,
    };
    *slot = Some(patch);
}
