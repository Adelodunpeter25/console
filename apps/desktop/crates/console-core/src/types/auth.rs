//! Auth DTOs mirroring the server's `/api/auth/*` contracts.
//!
//! The wire types are now shared protobuf (console_proto, see
//! proto/console/v1/auth.proto) and re-exported below. `OAuthProviderId`
//! stays a hand-written UI-side enum — it is a closed set of ids the UI
//! switches on, not something the server enumerates.

use serde::{Deserialize, Serialize};

pub use console_proto::{
    AuthStatusResponse, GitHubAuthStatus, GitHubLogoutResponse, OAuthCallbackResponse,
    OAuthLoginUrlResponse, ProjectIdResponse, ProviderAuthStatus,
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