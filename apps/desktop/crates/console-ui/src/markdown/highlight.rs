//! Syntax highlighting types backed by the `syntax` crate (Tree-sitter / Lumis).

use std::ops::Range;

pub use syntax::{Capture, Language, LanguageRegistry};

/// Token class mapping for styling code tokens and diff lines.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum TokenClass {
    Keyword,
    Literal,
    String,
    Comment,
    Number,
    Type,
    Function,
    Meta,
    Added,
    Removed,
}

impl From<syntax::Capture> for TokenClass {
    fn from(capture: syntax::Capture) -> Self {
        match capture {
            syntax::Capture::Keyword => Self::Keyword,
            syntax::Capture::String => Self::String,
            syntax::Capture::Comment => Self::Comment,
            syntax::Capture::Number => Self::Number,
            syntax::Capture::Function => Self::Function,
            syntax::Capture::Type => Self::Type,
            syntax::Capture::Plain => Self::Literal,
        }
    }
}

/// One highlighted span, as byte offsets.
#[derive(Clone, Debug, PartialEq)]
pub struct Token {
    pub range: Range<usize>,
    pub class: TokenClass,
}

/// Resolve a fenced-code info string or language tag to a Language definition.
pub fn lang_for_tag(tag: &str) -> Option<&'static Language> {
    let registry = LanguageRegistry::builtin();
    registry.for_name(tag).or_else(|| registry.for_extension(tag))
}

/// Resolve a file path to a language definition name.
pub fn lang_tag_for_path(path: &str) -> Option<&'static str> {
    LanguageRegistry::for_path(std::path::Path::new(path)).map(|l| l.name)
}
