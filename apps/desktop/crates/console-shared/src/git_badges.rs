//! Git working-tree state, reduced to what a file tree paints.
//!
//! One implementation, two callers: the desktop imports this crate directly,
//! Android reaches it over JNI. The rules live here so a file cannot be green
//! on one client and grey on the other.
//!
//! The palette does not live here: a badge carries a *role* — how loudly it
//! reports itself — and each client maps that role to its own colors.

use std::collections::HashMap;
use std::path::Path;

/// A file's Git state. Variants are ordered by how loudly a folder should
/// report them: a folder holding a deleted file and a new one shows the
/// deletion.
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub enum GitBadge {
    Untracked,
    Added,
    Renamed,
    Modified,
    Deleted,
    Conflict,
}

/// How loudly a badge reports itself. Each client maps a role to its own
/// palette, so the two cannot disagree about what a badge *means* even
/// though they draw it differently.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum GitBadgeRole {
    Success,
    Warning,
    Danger,
    Muted,
}

impl GitBadge {
    /// Maps the server's status code (`M`/`A`/`D`/`R`/`U`/`?`); anything else
    /// gets no badge rather than a guess.
    pub fn from_code(code: &str) -> Option<Self> {
        Some(match code {
            "?" => Self::Untracked,
            "A" => Self::Added,
            "R" | "C" => Self::Renamed,
            "M" => Self::Modified,
            "D" => Self::Deleted,
            "U" => Self::Conflict,
            _ => return None,
        })
    }

    /// The letter drawn beside the file.
    pub fn label(self) -> &'static str {
        match self {
            Self::Untracked => "U",
            Self::Added => "A",
            Self::Renamed => "R",
            Self::Modified => "M",
            Self::Deleted => "D",
            Self::Conflict => "!",
        }
    }

    /// How loudly this badge reports itself.
    pub fn role(self) -> GitBadgeRole {
        match self {
            Self::Untracked | Self::Added => GitBadgeRole::Success,
            Self::Renamed => GitBadgeRole::Muted,
            Self::Modified => GitBadgeRole::Warning,
            Self::Deleted | Self::Conflict => GitBadgeRole::Danger,
        }
    }
}

/// Every status code the badge rules know, so a client can map a code to its
/// badge in one call instead of asking for each one separately.
pub const GIT_BADGE_CODES: &[&str] = &["?", "A", "R", "C", "M", "D", "U"];

/// The badge for every code the rules know, in [`GIT_BADGE_CODES`] order.
pub fn git_badge_table() -> Vec<(&'static str, GitBadge)> {
    GIT_BADGE_CODES
        .iter()
        .filter_map(|code| GitBadge::from_code(code).map(|badge| (*code, badge)))
        .collect()
}

/// One changed file, as the badge logic needs it.
#[derive(Clone, Copy, Debug)]
pub struct ChangedFile<'a> {
    pub path: &'a str,
    pub status: &'a str,
}

/// Git badge per absolute path, with every changed file's ancestors carrying
/// the strongest badge beneath them (VS Code's folder tinting).
///
/// Built once per status update, not per frame. The walk up from a file stops
/// at the first ancestor already showing an equal or stronger badge: that
/// ancestor's own ancestors were raised to at least that level when it was
/// set, so the rest of the chain cannot change. A thousand files in one
/// folder therefore cost one walk, not a thousand.
pub fn aggregate_git_badges(files: &[ChangedFile<'_>]) -> HashMap<String, GitBadge> {
    let mut badges: HashMap<String, GitBadge> = HashMap::with_capacity(files.len() * 2);
    for file in files {
        let Some(badge) = GitBadge::from_code(file.status) else {
            continue;
        };
        raise(&mut badges, file.path, badge);
        for ancestor in Path::new(file.path).ancestors().skip(1) {
            if ancestor.file_name().is_none() {
                break;
            }
            let Some(path) = ancestor.to_str() else {
                break;
            };
            if let Some(existing) = badges.get_mut(path) {
                if *existing >= badge {
                    break;
                }
                *existing = badge;
            } else {
                badges.insert(path.to_owned(), badge);
            }
        }
    }
    badges
}

fn raise(badges: &mut HashMap<String, GitBadge>, path: &str, badge: GitBadge) {
    match badges.get_mut(path) {
        Some(existing) => *existing = (*existing).max(badge),
        None => {
            badges.insert(path.to_owned(), badge);
        }
    }
}
