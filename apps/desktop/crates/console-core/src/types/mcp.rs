use serde::de::{MapAccess, SeqAccess, Visitor};
use serde::{Deserialize, Deserializer, Serialize};
use std::fmt;

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum McpTransportType {
    Http,
    Stdio,
}

impl Default for McpTransportType {
    fn default() -> Self {
        Self::Stdio
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum McpAuthType {
    None,
    Static,
    Oauth2,
}

impl Default for McpAuthType {
    fn default() -> Self {
        Self::None
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum McpConnectionStatus {
    Disconnected,
    Connecting,
    Connected,
    NeedsAuth,
    Error(String),
}

impl Default for McpConnectionStatus {
    fn default() -> Self {
        Self::Disconnected
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct McpToolInfo {
    pub name: String,
    #[serde(default)]
    pub description: Option<String>,
}

/// One MCP server definition.
///
/// Deserialization goes through [`McpServerConfigWire`] because the server
/// spells three of these fields differently than the save payload does;
/// serialization stays derived so this type keeps writing the shape the
/// server accepts.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(from = "McpServerConfigWire")]
pub struct McpServerConfig {
    pub id: String,
    /// The server calls this `label`; it is spelled `name` here and in the
    /// save payload. Either key is accepted on the way in.
    pub name: String,
    pub transport: McpTransportType,
    pub url: Option<String>,
    pub auth_type: McpAuthType,
    pub command: Option<String>,
    pub args: Vec<String>,
    pub env: Vec<(String, String)>,
    pub status: McpConnectionStatus,
    pub auth_url: Option<String>,
    pub tools: Vec<McpToolInfo>,
}

/// The wire shape: every spelling either side might send. Only the fields
/// that are renamed or reshaped differ from [`McpServerConfig`]; the rest
/// carry over untouched.
#[derive(Deserialize)]
struct McpServerConfigWire {
    id: String,
    /// The client's spelling of `label`.
    #[serde(default)]
    name: Option<String>,
    /// The server's spelling of `name`.
    #[serde(default)]
    label: Option<String>,
    transport: McpTransportType,
    #[serde(default)]
    url: Option<String>,
    /// The client's flat auth-type string.
    #[serde(default)]
    auth_type: Option<McpAuthType>,
    /// The server's nested auth object.
    #[serde(default)]
    auth: Option<AuthObject>,
    #[serde(default)]
    command: Option<String>,
    #[serde(default)]
    args: Vec<String>,
    #[serde(default, deserialize_with = "de_env_pairs")]
    env: Vec<(String, String)>,
    /// The status arrives as a bare string on the list endpoint, with any
    /// message in a separate `error` member; this client serializes `Error`
    /// as an object. Accept both shapes.
    #[serde(default)]
    status: Option<StatusShape>,
    /// The server's status message, present only when status is `error`.
    #[serde(default)]
    error: Option<String>,
    #[serde(default, rename = "authUrl")]
    auth_url: Option<String>,
    #[serde(default)]
    tools: Vec<McpToolInfo>,
}

/// The server's `auth` member. Only the type is read; `tokenRef` is a server
/// concern and never round-trips back to the client.
#[derive(Deserialize)]
struct AuthObject {
    #[serde(rename = "type", default)]
    kind: Option<McpAuthType>,
}

/// A connection status in either encoding. Checked bare-string-first so the
/// server's `"connected"` never gets mistaken for an object.
#[derive(Deserialize)]
#[serde(untagged)]
enum StatusShape {
    /// `"disconnected"`, `"connecting"`, `"connected"`, `"needs_auth"`,
    /// `"error"` — what `GET /api/mcp/servers` sends.
    Bare(String),
    /// `{"error": "..."}` — this client's encoding of
    /// [`McpConnectionStatus::Error`].
    Tagged { error: String },
}

impl StatusShape {
    /// Fold the status and the server's separate message into one variant.
    /// Anything unrecognised becomes an error rather than a decode failure:
    /// one server in a bad state must not blank the whole list.
    fn resolve(self, message: Option<String>) -> McpConnectionStatus {
        let message = message.filter(|message| !message.is_empty());
        match self {
            StatusShape::Tagged { error } => McpConnectionStatus::Error(error),
            StatusShape::Bare(raw) => match raw.as_str() {
                "disconnected" => McpConnectionStatus::Disconnected,
                "connecting" => McpConnectionStatus::Connecting,
                "connected" => McpConnectionStatus::Connected,
                "needs_auth" => McpConnectionStatus::NeedsAuth,
                other => McpConnectionStatus::Error(message.unwrap_or_else(|| {
                    // The server reported `error` with no detail; a bare word
                    // is not a useful message to show the user.
                    if other == "error" {
                        "connection failed".to_string()
                    } else {
                        other.to_string()
                    }
                })),
            },
        }
    }
}

impl From<McpServerConfigWire> for McpServerConfig {
    fn from(wire: McpServerConfigWire) -> Self {
        // An explicit auth spelling wins over the other; they are the same
        // setting, so a payload carrying both is still unambiguous.
        let auth_type = wire
            .auth_type
            .or_else(|| wire.auth.and_then(|auth| auth.kind))
            .unwrap_or_default();
        let name = wire
            .name
            .filter(|name| !name.is_empty())
            .or(wire.label)
            .unwrap_or_default();
        Self {
            id: wire.id,
            name,
            transport: wire.transport,
            url: wire.url,
            auth_type,
            command: wire.command,
            args: wire.args,
            env: wire.env,
            // Absent status means the server never reported one; treat that
            // as the default rather than an error.
            status: match wire.status {
                Some(shape) => shape.resolve(wire.error),
                None => McpConnectionStatus::default(),
            },
            auth_url: wire.auth_url,
            tools: wire.tools,
        }
    }
}

/// Decodes `env` from either shape: the object form the server stores and
/// returns (`{"KEY":"VAL"}`) or the pair-array form this client serializes
/// (`[["KEY","VAL"], ...]`). Object key order is preserved so the settings
/// list renders in a stable order.
fn de_env_pairs<'de, D>(deserializer: D) -> Result<Vec<(String, String)>, D::Error>
where
    D: Deserializer<'de>,
{
    struct EnvVisitor;

    impl<'de> Visitor<'de> for EnvVisitor {
        type Value = Vec<(String, String)>;

        fn expecting(&self, f: &mut fmt::Formatter) -> fmt::Result {
            f.write_str("an object of string values or an array of [key, value] pairs")
        }

        fn visit_seq<A>(self, mut seq: A) -> Result<Self::Value, A::Error>
        where
            A: SeqAccess<'de>,
        {
            let mut pairs = Vec::new();
            while let Some(pair) = seq.next_element::<(String, String)>()? {
                pairs.push(pair);
            }
            Ok(pairs)
        }

        fn visit_map<A>(self, mut map: A) -> Result<Self::Value, A::Error>
        where
            A: MapAccess<'de>,
        {
            let mut pairs = Vec::new();
            while let Some(pair) = map.next_entry::<String, String>()? {
                pairs.push(pair);
            }
            Ok(pairs)
        }
    }

    deserializer.deserialize_any(EnvVisitor)
}
