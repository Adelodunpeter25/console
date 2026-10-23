// Pure string helpers shared with Android live in `console-shared`; these
// re-exports keep the old module paths.
pub use console_shared::markdown::{file_links, mend};
pub mod highlight;
pub mod math;
pub mod parser;
pub mod render;
pub mod selection;
pub mod veil;

pub use render::{
    MarkdownView, Metrics, Palette, markdown, render_markdown, render_markdown_block,
};
