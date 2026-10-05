// Canonical wire types from the shared protobuf schema
// (proto/console/v1/fs.proto): trimmed to what the UIs render, grep counts
// narrowed to u32 so they stay JSON numbers, sizes as uint64 strings.
//
// The grep query enums stay hand-written: modes travel as URL query strings,
// never JSON bodies — they are UI toggle state with a documented vocabulary,
// not schema. Same for GrepOptions, which builds those query strings.

pub use console_proto::{
    DirCreateResponse, DirDeleteResponse, FileDeleteResponse, FileSearchResult, FileWriteResponse,
    FsBrowseResult, FsChangeEvent, FsDirectoryTree, FsFileContent, FsTreeEntry, GrepMatch,
    GrepMatchRange, GrepResult, WriteFileRequest, CreateDirRequest,
};

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
