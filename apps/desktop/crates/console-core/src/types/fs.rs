use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FsEntry {
    pub name: String,
    pub path: String,
    /// Matches the server's `isDir` field (same as [`FsTreeEntry`]).
    pub is_dir: bool,
    pub size: Option<u64>,
    pub modified_at: Option<i64>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FsTreeEntry {
    pub name: String,
    pub path: String,
    pub is_dir: bool,
    pub size: Option<u64>,
    pub git_status: Option<String>,
    pub children: Option<Vec<FsTreeEntry>>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BrowseDirectoryResponse {
    pub current_path: String,
    pub parent_path: Option<String>,
    pub entries: Vec<FsEntry>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DirectoryTreeResponse {
    pub path: String,
    pub tree_formatted: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FileContentResponse {
    pub path: String,
    pub content: String,
    pub is_binary: Option<bool>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WriteFileDto {
    pub path: String,
    pub content: String,
}

/// Half-open byte range within [`GrepMatch::line_content`], used to bold the
/// matched substring in place (offsets, not rune/char indexes, matching the
/// server's fff-backed byte offsets).
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct GrepMatchRange {
    pub start: usize,
    pub end: usize,
}

/// One content-search hit from `GET /api/fs/grep`, shaped for the global
/// search panel: grouping by `rel_path`/`file_name`, highlighting via
/// `match_ranges`.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GrepMatch {
    pub rel_path: String,
    pub file_name: String,
    pub line_number: u64,
    pub column: i64,
    pub end_column: i64,
    pub line_content: String,
    pub match_ranges: Vec<GrepMatchRange>,
    #[serde(default)]
    pub is_binary: bool,
    #[serde(default)]
    pub is_definition: bool,
}

/// Response body of `GET /api/fs/grep`.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GrepResult {
    pub matches: Vec<GrepMatch>,
    pub total_matched: usize,
    pub files_searched: usize,
    pub total_files: usize,
    pub filtered_files: usize,
    pub next_cursor: u32,
    pub has_more: bool,
    /// Set when the query was an invalid regex and fff fell back to a
    /// literal search; results may still be usable.
    pub regex_error: Option<String>,
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

