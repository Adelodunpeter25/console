pub mod blocked_panel;
pub mod code_viewer;
pub mod diff_viewer;
pub mod file_viewer;
pub mod image_preview;
pub mod logs_viewer;
pub mod markdown_viewer;
pub mod svg;

pub use blocked_panel::BlockedFilePanel;
pub use code_viewer::{
    CODE_LINE_HEIGHT, CODE_VIEWER_CONTEXT, CodePosition, CodeSelection, CodeViewer, CodeViewerLine,
    SelectionState, build_diff_lines, build_file_lines, floor_selection_range,
    init_code_viewer_keybindings,
};
pub use diff_viewer::DiffViewer;
pub use file_viewer::FileViewer;
pub use logs_viewer::{ESTIMATED_LOG_ROW_HEIGHT, LogsViewer};
pub use image_preview::{
    ImageMeta, ImagePreview, SvgViewMode, format_bytes, image_dimensions, svg_source_header,
};
pub use markdown_viewer::MarkdownViewer;
pub use svg::rasterize_svg;
pub use editor_ui::{
    DiffState, DiffView as EditorDiffView, EditorState, EditorView, FontConfig, IndentOptions,
    Mode as EditorMode, SearchState as EditorSearchState, Selection as EditorSelection,
};
pub use syntax::{
    Capture as SyntaxCapture, HighlightSpan, Language, LanguageRegistry,
    ThemePreset as SyntaxThemePreset,
};
