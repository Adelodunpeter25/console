// Canonical wire types from the shared protobuf schema
// (proto/console/v1/fs.proto): trimmed to what the UIs render, grep counts
// narrowed to u32 so they stay JSON numbers, sizes as uint64 strings.
//
// The grep query enums stay hand-written: modes travel as URL query strings,
// never JSON bodies — they are UI toggle state with a documented vocabulary,
// not schema. Same for GrepOptions, which builds those query strings.

pub use console_proto::{
    CreateDirRequest, DirCreateResponse, DirDeleteResponse, FileDeleteResponse, FileSearchResult,
    FsBrowseResult, FsChangeEvent, FsDirectoryTree, FsFileContent, FsTreeEntry, GrepMatch,
    GrepMatchRange, GrepResult, SaveFileRequest, SaveFileResponse,
};

/// Result of a successful `POST /api/fs/save`.
pub type FsSaveOutcome = SaveFileResponse;

/// A file's content together with the ETag it was loaded at.
#[derive(Clone, Debug)]
pub struct FsFileWithEtag {
    pub content: FsFileContent,
    pub etag: Option<String>,
}

/// Failure of `FsService::save_file`, split so the caller can tell a stale
/// buffer apart from a transport or server error without string matching.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum FsSaveError {
    /// The file changed on disk since this buffer was loaded. Nothing was
    /// written. `current_etag` is the file's ETag now, when the server sent it.
    FileChanged { current_etag: Option<String> },
    Other(String),
}

impl std::fmt::Display for FsSaveError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::FileChanged { .. } => {
                f.write_str("File changed on disk since you opened it. Your edits were not saved.")
            }
            Self::Other(message) => f.write_str(message),
        }
    }
}

impl std::error::Error for FsSaveError {}

/// The 409 envelope: the route puts `code` and `etag` beside `error`, not in
/// `data`, so this is parsed on its own rather than through `ApiResponse`.
#[derive(serde::Deserialize)]
pub(crate) struct SaveConflictEnvelope {
    #[serde(default)]
    pub etag: Option<String>,
}

/// Content-search matching mode, mirrors the server's `fff.GrepMode`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum GrepMode {
    Plain,
    #[default]
    Regex,
    Fuzzy,
}

impl GrepMode {
    pub fn as_query_value(self) -> &'static str {
        match self {
            Self::Plain => "plain",
            Self::Regex => "regex",
            Self::Fuzzy => "fuzzy",
        }
    }
}

/// Case-matching mode, mirrors the server's `fff.CaseMode`. Maps to the
/// search panel's "Aa" toggle (sensitive/insensitive) with smart as the
/// untoggled default.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum GrepCaseMode {
    #[default]
    Smart,
    Sensitive,
    Insensitive,
}

impl GrepCaseMode {
    pub fn as_query_value(self) -> &'static str {
        match self {
            Self::Smart => "smart",
            Self::Sensitive => "sensitive",
            Self::Insensitive => "insensitive",
        }
    }
}

/// Query parameters for [`crate::services::FsService::grep`], mirroring the
/// search panel's Aa / ab / .* toggles plus paging.
#[derive(Clone, Debug, Default)]
pub struct GrepOptions {
    pub mode: GrepMode,
    pub case_mode: GrepCaseMode,
    pub whole_word: bool,
    pub context_lines: Option<u32>,
    pub max_matches: Option<u32>,
    pub cursor: Option<u32>,
}
