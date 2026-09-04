//! Block-level markdown parsing over `pulldown-cmark`.
//!
//! A full parse produces a [`BlockTree`]: top-level blocks paired with their
//! byte ranges in the source. The range start of the last top-level block is a
//! *stable boundary* — appending to the source cannot change anything before
//! it — which is what [`IncrementalParser`] exploits so a streamed delta costs
//! roughly O(delta + last block) instead of O(document).
//!
//! Soundness guard: link reference definitions (`[label]: url`) resolve
//! non-locally, so a source containing one drops back to full reparses.

use std::ops::Range;
use std::sync::LazyLock;

use pulldown_cmark::{Alignment, CodeBlockKind, Event, HeadingLevel, Options, Parser, Tag};
use regex::Regex;

/// CommonMark only recognizes angle-bracket autolinks. Transcript content is
/// conversational, so bare web URLs should be useful without requiring the
/// author to write `<https://...>` or `[label](https://...)`.
static BARE_WEB_URL: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r#"(?i)\bhttps?://[^\s<>"`\\]+"#).expect("bare web URL regex should compile")
});

/// Bare `file://` URLs. Resolved by the app-owned `LinkHandler` into a
/// workspace tab instead of a browser navigation.
static BARE_FILE_URL: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r#"(?i)\bfile://[^\s<>"`\\]+"#).expect("bare file URL regex should compile")
});

/// Bare file paths (`apps/server/index.ts`, `./src/foo.rs:12:3`,
/// `/abs/path/file.md`, `~/proj/file.ts`). Requires a file extension and —
/// to avoid matching prose like `v1.2` — a `/` somewhere in the match, or a
/// leading `~/`, `/`, `./`, `../` prefix. An optional `:line` / `:line:col`
/// suffix is included so the text stays intact; opening strips it for now.
static BARE_FILE_PATH: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r#"\b(?:~/|/|\./|\.\./)?(?:[\w\-.]+/)*[\w\-.]*[\w]\.[A-Za-z0-9]{1,10}(?::\d+(?::\d+)?)?"#)
        .expect("bare file path regex should compile")
});

// ── Tree model ─────────────────────────────────────────────────────────────

/// Inline styling threaded through nested emphasis and links.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct InlineStyle {
    pub bold: bool,
    pub italic: bool,
    pub code: bool,
    pub strikethrough: bool,
    /// Destination URL when inside a link.
    pub link: Option<String>,
}

/// One run of identically styled inline text.
#[derive(Clone, Debug, PartialEq)]
pub struct InlineRun {
    pub text: String,
    pub style: InlineStyle,
}

impl InlineRun {
    fn plain(text: impl Into<String>) -> Self {
        Self {
            text: text.into(),
            style: InlineStyle::default(),
        }
    }
}

/// GFM column alignment. Unspecified renders as `Left`.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum TableAlign {
    #[default]
    Left,
    Center,
    Right,
}

/// A list item: its blocks plus an optional task-list checkbox state.
#[derive(Clone, Debug, PartialEq)]
pub struct ListItem {
    pub task: Option<bool>,
    pub blocks: Vec<Block>,
}

/// One piece of inline content. Images interrupt a run of text rather than
/// styling it, so they cannot be an [`InlineStyle`] flag.
#[derive(Clone, Debug, PartialEq)]
enum InlinePiece {
    Run(InlineRun),
    Image { url: String, alt: String },
}

/// A markdown block. Containers nest.
#[derive(Clone, Debug, PartialEq)]
pub enum Block {
    Paragraph {
        runs: Vec<InlineRun>,
    },
    /// A standalone image. Inline images split their paragraph so the text
    /// before and after keeps its order around them.
    Image {
        url: String,
        alt: String,
    },
    Heading {
        level: u8,
        runs: Vec<InlineRun>,
    },
    CodeBlock {
        language: Option<String>,
        code: String,
    },
    BlockQuote {
        children: Vec<Block>,
    },
    List {
        /// `Some(n)` for an ordered list starting at `n`.
        ordered_start: Option<u64>,
        items: Vec<ListItem>,
    },
    Table {
        header: Vec<Vec<InlineRun>>,
        rows: Vec<Vec<Vec<InlineRun>>>,
        align: Vec<TableAlign>,
    },
    Rule,
}

/// A top-level block plus its byte range in the source.
#[derive(Clone, Debug, PartialEq)]
pub struct TopBlock {
    pub range: Range<usize>,
    pub block: Block,
}

/// Top-level blocks in document order.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct BlockTree {
    pub blocks: Vec<TopBlock>,
}

// ── Full parse ─────────────────────────────────────────────────────────────

fn options() -> Options {
    Options::ENABLE_TABLES | Options::ENABLE_STRIKETHROUGH | Options::ENABLE_TASKLISTS
}

/// Parse a whole source into a [`BlockTree`].
pub fn parse(source: &str) -> BlockTree {
    let events = Parser::new_ext(source, options())
        .into_offset_iter()
        .collect::<Vec<_>>();
    let mut cursor = Cursor {
        events: &events,
        index: 0,
    };
    let mut blocks = Vec::new();
    while let Some((event, range)) = cursor.peek() {
        let range = range.clone();
        match event {
            Event::Rule => {
                cursor.bump();
                blocks.push(TopBlock {
                    range,
                    block: Block::Rule,
                });
            }
            Event::Start(_) => {
                for block in parse_started_block(&mut cursor) {
                    blocks.push(TopBlock {
                        range: range.clone(),
                        block,
                    });
                }
            }
            // Stray inline events at the top level should not occur; skip them
            // rather than looping forever.
            _ => cursor.bump(),
        }
    }
    BlockTree { blocks }
}

struct Cursor<'a, 'e> {
    events: &'a [(Event<'e>, Range<usize>)],
    index: usize,
}

impl<'e> Cursor<'_, 'e> {
    fn peek(&self) -> Option<&(Event<'e>, Range<usize>)> {
        self.events.get(self.index)
    }

    fn peek_event(&self) -> Option<&Event<'e>> {
        self.peek().map(|(event, _)| event)
    }

    fn bump(&mut self) {
        self.index += 1;
    }

    fn next_event(&mut self) -> Option<Event<'e>> {
        let event = self.events.get(self.index).map(|(event, _)| event.clone());
        if event.is_some() {
            self.index += 1;
        }
        event
    }
}

fn is_block_tag(tag: &Tag) -> bool {
    matches!(
        tag,
        Tag::Paragraph
            | Tag::Heading { .. }
            | Tag::CodeBlock(_)
            | Tag::BlockQuote(_)
            | Tag::List(_)
            | Tag::Item
            | Tag::Table(_)
            | Tag::HtmlBlock
            | Tag::FootnoteDefinition(_)
    )
}

fn heading_level(level: HeadingLevel) -> u8 {
    match level {
        HeadingLevel::H1 => 1,
        HeadingLevel::H2 => 2,
        HeadingLevel::H3 => 3,
        HeadingLevel::H4 => 4,
        HeadingLevel::H5 => 5,
        HeadingLevel::H6 => 6,
    }
}

/// Consume a `Start(tag)` and everything through its matching `End`. Unknown
/// containers are transparent: their children splice into the parent.
fn parse_started_block(cursor: &mut Cursor) -> Vec<Block> {
    let Some(Event::Start(tag)) = cursor.next_event() else {
        return Vec::new();
    };
    match tag {
        Tag::Paragraph => pieces_into_blocks(parse_inline_container(cursor)),
        Tag::Heading { level, .. } => vec![Block::Heading {
            level: heading_level(level),
            runs: pieces_into_runs(parse_inline_container(cursor)),
        }],
        Tag::CodeBlock(kind) => {
            let language = match kind {
                CodeBlockKind::Fenced(info) => info
                    .split_whitespace()
                    .next()
                    .filter(|language| !language.is_empty())
                    .map(str::to_owned),
                CodeBlockKind::Indented => None,
            };
            let mut code = String::new();
            loop {
                match cursor.next_event() {
                    Some(Event::Text(text)) => code.push_str(&text),
                    Some(Event::End(_)) | None => break,
                    Some(_) => {}
                }
            }
            // Fenced blocks carry a trailing newline; rendering is per line so
            // the empty final line would otherwise add a phantom row.
            if code.ends_with('\n') {
                code.pop();
            }
            vec![Block::CodeBlock { language, code }]
        }
        Tag::BlockQuote(_) => vec![Block::BlockQuote {
            children: parse_block_sequence(cursor),
        }],
        Tag::List(ordered_start) => {
            let mut items = Vec::new();
            loop {
                match cursor.peek_event() {
                    Some(Event::Start(Tag::Item)) => {
                        cursor.bump();
                        items.push(parse_list_item(cursor));
                    }
                    Some(Event::End(_)) | None => {
                        cursor.bump();
                        break;
                    }
                    Some(_) => cursor.bump(),
                }
            }
            vec![Block::List {
                ordered_start,
                items,
            }]
        }
        Tag::Table(alignments) => {
            let align = alignments
                .iter()
                .map(|alignment| match alignment {
                    Alignment::Center => TableAlign::Center,
                    Alignment::Right => TableAlign::Right,
                    Alignment::None | Alignment::Left => TableAlign::Left,
                })
                .collect();
            vec![parse_table(cursor, align)]
        }
        Tag::HtmlBlock => {
            // Raw HTML renders literally: an agent transcript is far more
            // likely to be *discussing* markup than asking us to apply it.
            let mut text = String::new();
            loop {
                match cursor.next_event() {
                    Some(Event::Html(chunk) | Event::Text(chunk)) => text.push_str(&chunk),
                    Some(Event::End(_)) | None => break,
                    Some(_) => {}
                }
            }
            let text = text.trim_end_matches('\n').to_owned();
            if text.is_empty() {
                Vec::new()
            } else {
                vec![Block::Paragraph {
                    runs: vec![InlineRun::plain(text)],
                }]
            }
        }
        // Transparent containers (footnote definitions, and anything a future
        // pulldown-cmark adds).
        _ => parse_block_sequence(cursor),
    }
}

/// Parse a list item, lifting a leading task-list marker out of its content.
fn parse_list_item(cursor: &mut Cursor) -> ListItem {
    let task = match cursor.peek_event() {
        Some(Event::TaskListMarker(checked)) => {
            let checked = *checked;
            cursor.bump();
            Some(checked)
        }
        _ => None,
    };
    ListItem {
        task,
        blocks: parse_block_sequence(cursor),
    }
}

/// Parse blocks until the container's `End` (which is consumed). Bare inline
/// events — tight list items — accumulate into an implicit paragraph.
fn parse_block_sequence(cursor: &mut Cursor) -> Vec<Block> {
    let mut blocks: Vec<Block> = Vec::new();
    let mut inline: Vec<InlinePiece> = Vec::new();
    while let Some(event) = cursor.peek_event() {
        match event {
            Event::End(_) => {
                cursor.bump();
                break;
            }
            Event::Start(tag) if is_block_tag(tag) => {
                flush_paragraph(&mut blocks, &mut inline);
                blocks.extend(parse_started_block(cursor));
            }
            Event::Rule => {
                flush_paragraph(&mut blocks, &mut inline);
                cursor.bump();
                blocks.push(Block::Rule);
            }
            _ => parse_inline_event(cursor, &mut inline, &InlineStyle::default()),
        }
    }
    flush_paragraph(&mut blocks, &mut inline);
    blocks
}

fn flush_paragraph(blocks: &mut Vec<Block>, inline: &mut Vec<InlinePiece>) {
    if !inline.is_empty() {
        blocks.extend(pieces_into_blocks(merge_pieces(std::mem::take(inline))));
    }
}

fn parse_table(cursor: &mut Cursor, align: Vec<TableAlign>) -> Block {
    let mut header = Vec::new();
    let mut rows = Vec::new();
    loop {
        match cursor.peek_event() {
            Some(Event::Start(Tag::TableHead)) => {
                cursor.bump();
                header = parse_table_row(cursor);
            }
            Some(Event::Start(Tag::TableRow)) => {
                cursor.bump();
                rows.push(parse_table_row(cursor));
            }
            Some(Event::End(_)) | None => {
                cursor.bump();
                break;
            }
            Some(_) => cursor.bump(),
        }
    }
    Block::Table {
        header,
        rows,
        align,
    }
}

fn parse_table_row(cursor: &mut Cursor) -> Vec<Vec<InlineRun>> {
    let mut cells = Vec::new();
    loop {
        match cursor.peek_event() {
            Some(Event::Start(Tag::TableCell)) => {
                cursor.bump();
                // A table cell is one line of text; an image there degrades to
                // its alt rather than breaking the row's geometry.
                cells.push(pieces_into_runs(parse_inline_container(cursor)));
            }
            Some(Event::End(_)) | None => {
                cursor.bump();
                break;
            }
            Some(_) => cursor.bump(),
        }
    }
    cells
}

/// Collect inline pieces until the container's `End` (which is consumed).
fn parse_inline_container(cursor: &mut Cursor) -> Vec<InlinePiece> {
    let mut pieces = Vec::new();
    while let Some(event) = cursor.peek_event() {
        if matches!(event, Event::End(_)) {
            cursor.bump();
            break;
        }
        parse_inline_event(cursor, &mut pieces, &InlineStyle::default());
    }
    merge_pieces(pieces)
}

/// Split inline pieces into blocks, so images become their own block and the
/// text around them keeps its order.
fn pieces_into_blocks(pieces: Vec<InlinePiece>) -> Vec<Block> {
    let mut blocks = Vec::new();
    let mut runs: Vec<InlineRun> = Vec::new();
    for piece in pieces {
        match piece {
            InlinePiece::Run(run) => runs.push(run),
            InlinePiece::Image { url, alt } => {
                if !runs.is_empty() {
                    blocks.push(Block::Paragraph {
                        runs: std::mem::take(&mut runs),
                    });
                }
                blocks.push(Block::Image { url, alt });
            }
        }
    }
    if !runs.is_empty() {
        blocks.push(Block::Paragraph { runs });
    }
    blocks
}

/// Flatten pieces to runs for contexts that cannot host a block-level image.
fn pieces_into_runs(pieces: Vec<InlinePiece>) -> Vec<InlineRun> {
    merge_runs(
        pieces
            .into_iter()
            .map(|piece| match piece {
                InlinePiece::Run(run) => run,
                InlinePiece::Image { alt, .. } => InlineRun::plain(alt),
            })
            .collect(),
    )
}

/// Consume one inline event, appending its runs with `style` applied. Nested
/// emphasis and links recurse with an extended style.
fn parse_inline_event(cursor: &mut Cursor, pieces: &mut Vec<InlinePiece>, style: &InlineStyle) {
    let Some(event) = cursor.next_event() else {
        return;
    };
    let mut push_run = |run: InlineRun| pieces.push(InlinePiece::Run(run));
    match event {
        Event::Text(text) => push_run(InlineRun {
            text: text.to_string(),
            style: style.clone(),
        }),
        Event::Code(text) => {
            let mut style = style.clone();
            style.code = true;
            push_run(InlineRun {
                text: text.to_string(),
                style,
            });
        }
        // A hard or soft break inside a paragraph is a line break in the
        // rendered run: shaped text splits on '\n' on its own.
        Event::SoftBreak | Event::HardBreak => push_run(InlineRun {
            text: "\n".to_owned(),
            style: style.clone(),
        }),
        Event::Start(Tag::Image {
            dest_url, title, ..
        }) => {
            // The image's children are its alt text.
            let mut alt_pieces = Vec::new();
            while let Some(event) = cursor.peek_event() {
                if matches!(event, Event::End(_)) {
                    cursor.bump();
                    break;
                }
                parse_inline_event(cursor, &mut alt_pieces, &InlineStyle::default());
            }
            let alt = pieces_into_runs(alt_pieces)
                .into_iter()
                .map(|run| run.text)
                .collect::<String>();
            let alt = if alt.trim().is_empty() {
                title.to_string()
            } else {
                alt
            };
            pieces.push(InlinePiece::Image {
                url: dest_url.to_string(),
                alt,
            });
        }
        Event::Start(tag) => {
            let mut nested = style.clone();
            match &tag {
                Tag::Emphasis => nested.italic = true,
                Tag::Strong => nested.bold = true,
                Tag::Strikethrough => nested.strikethrough = true,
                Tag::Link { dest_url, .. } => nested.link = Some(dest_url.to_string()),
                _ => {}
            }
            while let Some(event) = cursor.peek_event() {
                if matches!(event, Event::End(_)) {
                    cursor.bump();
                    break;
                }
                parse_inline_event(cursor, pieces, &nested);
            }
        }
        // Inline HTML renders literally, matching the block-level choice.
        Event::Html(text) | Event::InlineHtml(text) => push_run(InlineRun {
            text: text.to_string(),
            style: style.clone(),
        }),
        Event::FootnoteReference(label) => push_run(InlineRun {
            text: format!("[{label}]"),
            style: style.clone(),
        }),
        Event::TaskListMarker(checked) => push_run(InlineRun {
            text: if checked { "[x] " } else { "[ ] " }.to_owned(),
            style: style.clone(),
        }),
        Event::End(_) | Event::Rule | Event::InlineMath(_) | Event::DisplayMath(_) => {}
    }
}

/// Sentence punctuation and unmatched closing delimiters are prose around a
/// URL, not part of it. Balanced delimiters remain valid URL characters, as in
/// Wikipedia paths ending in `(disambiguation)`.
fn trimmed_bare_url_end(text: &str, start: usize, candidate_end: usize) -> usize {
    let mut end = candidate_end;
    let mut parens = delimiter_balance(&text[start..end], '(', ')');
    let mut brackets = delimiter_balance(&text[start..end], '[', ']');
    let mut braces = delimiter_balance(&text[start..end], '{', '}');
    loop {
        let Some((offset, last)) = text[start..end].char_indices().next_back() else {
            return start;
        };
        let last_index = start + offset;
        let should_trim = match last {
            '.' | ',' | ':' | ';' | '?' | '!' | '\'' => true,
            ')' if parens < 0 => {
                parens += 1;
                true
            }
            ']' if brackets < 0 => {
                brackets += 1;
                true
            }
            '}' if braces < 0 => {
                braces += 1;
                true
            }
            _ => false,
        };
        if !should_trim {
            return end;
        }
        end = last_index;
    }
}

fn delimiter_balance(text: &str, open: char, close: char) -> i32 {
    text.chars().fold(0, |balance, character| {
        balance + i32::from(character == open) - i32::from(character == close)
    })
}

/// Coalesce neighbouring runs that share a style, leaving images in place.
fn merge_pieces(pieces: Vec<InlinePiece>) -> Vec<InlinePiece> {
    let mut merged: Vec<InlinePiece> = Vec::with_capacity(pieces.len());
    for piece in pieces {
        match piece {
            InlinePiece::Run(run) if run.text.is_empty() => {}
            InlinePiece::Run(run) => match merged.last_mut() {
                Some(InlinePiece::Run(last)) if last.style == run.style => {
                    last.text.push_str(&run.text)
                }
                _ => merged.push(InlinePiece::Run(run)),
            },
            image => merged.push(image),
        }
    }
    linkify_bare_urls(merged)
}

/// Linkify after Markdown has produced and merged its inline runs. Pulldown
/// can split ordinary text at potential emphasis punctuation inside a URL;
/// merging first lets the detector recover the whole displayed target.
fn linkify_bare_urls(pieces: Vec<InlinePiece>) -> Vec<InlinePiece> {
    let mut linked = Vec::with_capacity(pieces.len());
    for piece in pieces {
        match piece {
            InlinePiece::Run(run) if !run.style.code && run.style.link.is_none() => {
                push_linkified_run(run, &mut linked);
            }
            piece => linked.push(piece),
        }
    }
    linked
}

fn push_linkified_run(run: InlineRun, pieces: &mut Vec<InlinePiece>) {
    let mut cursor = 0;
    for candidate in BARE_WEB_URL.find_iter(&run.text) {
        let end = trimmed_bare_url_end(&run.text, candidate.start(), candidate.end());
        if end <= candidate.start() {
            continue;
        }
        if cursor < candidate.start() {
            push_file_linkified_segment(&run.text[cursor..candidate.start()], &run.style, pieces);
        }

        let url = &run.text[candidate.start()..end];
        let mut link_style = run.style.clone();
        link_style.link = Some(url.to_owned());
        pieces.push(InlinePiece::Run(InlineRun {
            text: url.to_owned(),
            style: link_style,
        }));
        cursor = end;
    }

    if cursor < run.text.len() {
        push_file_linkified_segment(&run.text[cursor..], &run.style, pieces);
    }
}

/// Linkify `file://` URLs and bare file paths inside a plain-text segment.
/// Web URLs were split out first, so this only sees the gaps between them.
fn push_file_linkified_segment(text: &str, style: &InlineStyle, pieces: &mut Vec<InlinePiece>) {
    let mut cursor = 0;
    for candidate in BARE_FILE_URL.find_iter(text) {
        let end = trimmed_bare_url_end(text, candidate.start(), candidate.end());
        if end <= candidate.start() {
            continue;
        }
        if cursor < candidate.start() {
            push_file_path_segment(&text[cursor..candidate.start()], style, pieces);
        }
        let url = &text[candidate.start()..end];
        let mut link_style = style.clone();
        link_style.link = Some(url.to_owned());
        pieces.push(InlinePiece::Run(InlineRun {
            text: url.to_owned(),
            style: link_style,
        }));
        cursor = end;
    }
    if cursor < text.len() {
        push_file_path_segment(&text[cursor..], style, pieces);
    }
}

fn push_file_path_segment(text: &str, style: &InlineStyle, pieces: &mut Vec<InlinePiece>) {
    let mut cursor = 0;
    for candidate in BARE_FILE_PATH.find_iter(text) {
        let matched = candidate.as_str();
        if !is_bare_file_path_match(matched) {
            continue;
        }
        let start = candidate.start();
        let end = candidate.end();
        if cursor < start {
            pieces.push(InlinePiece::Run(InlineRun {
                text: text[cursor..start].to_owned(),
                style: style.clone(),
            }));
        }
        let mut link_style = style.clone();
        link_style.link = Some(matched.to_owned());
        pieces.push(InlinePiece::Run(InlineRun {
            text: matched.to_owned(),
            style: link_style,
        }));
        cursor = end;
    }
    if cursor < text.len() {
        pieces.push(InlinePiece::Run(InlineRun {
            text: text[cursor..].to_owned(),
            style: style.clone(),
        }));
    }
}

/// Guard against prose like `v1.2`: a bare match only counts when it looks
/// like a path — a `/` somewhere, or an explicit `~/`, `/`, `./`, `../`
/// prefix that the regex already captured.
fn is_bare_file_path_match(matched: &str) -> bool {
    matched.starts_with("~/")
        || matched.starts_with('/')
        || matched.starts_with("./")
        || matched.starts_with("../")
        || matched.contains('/')
}

/// Coalesce neighbouring runs that share a style, so shaping sees the fewest
/// possible `TextRun`s.
fn merge_runs(runs: Vec<InlineRun>) -> Vec<InlineRun> {
    let mut merged: Vec<InlineRun> = Vec::with_capacity(runs.len());
    for run in runs {
        if run.text.is_empty() {
            continue;
        }
        match merged.last_mut() {
            Some(last) if last.style == run.style => last.text.push_str(&run.text),
            _ => merged.push(run),
        }
    }
    merged
}

// ── Incremental parse ──────────────────────────────────────────────────────

/// Streaming parser over an append-only source.
///
/// Keeps the settled prefix of top-level blocks and reparses only the tail from
/// the last stable boundary. `display_tree` additionally mends hanging inline
/// markers (see [`super::mend`]) so a closing `**` never reflows text that is
/// already on screen.
pub struct IncrementalParser {
    text: String,
    tree: BlockTree,
    /// Blocks before this index are settled: no append can change them.
    stable_prefix: usize,
    /// A link reference definition anywhere forces full reparses.
    full_reparse_only: bool,
}

impl Default for IncrementalParser {
    fn default() -> Self {
        Self::new()
    }
}

impl IncrementalParser {
    pub fn new() -> Self {
        Self {
            text: String::new(),
            tree: BlockTree::default(),
            stable_prefix: 0,
            full_reparse_only: false,
        }
    }

    pub fn text(&self) -> &str {
        &self.text
    }

    pub fn tree(&self) -> &BlockTree {
        &self.tree
    }

    /// Point the parser at `text`. Appends reparse incrementally; any other
    /// change falls back to a full reparse.
    pub fn set_text(&mut self, text: &str) {
        if text == self.text {
            return;
        }
        match text.strip_prefix(self.text.as_str()) {
            Some(delta) if !self.text.is_empty() && !self.full_reparse_only => {
                let delta = delta.to_owned();
                self.append(&delta);
            }
            _ => self.reset(text),
        }
    }

    /// Discard all state and parse `text` from scratch.
    pub fn reset(&mut self, text: &str) {
        self.text = text.to_owned();
        self.tree = parse(&self.text);
        self.full_reparse_only = has_link_definition(&self.text);
        self.stable_prefix = self.settled_prefix();
    }

    /// Append `delta`, reparsing only from the last stable block boundary.
    pub fn append(&mut self, delta: &str) {
        if delta.is_empty() {
            return;
        }
        if self.full_reparse_only {
            let mut text = std::mem::take(&mut self.text);
            text.push_str(delta);
            self.reset(&text);
            return;
        }

        let boundary = self
            .tree
            .blocks
            .get(self.stable_prefix)
            .map_or(self.text.len(), |block| block.range.start);
        self.text.push_str(delta);
        if has_link_definition(delta) {
            let text = std::mem::take(&mut self.text);
            self.reset(&text);
            return;
        }

        let tail = parse(&self.text[boundary..]);
        self.tree.blocks.truncate(self.stable_prefix);
        self.tree
            .blocks
            .extend(tail.blocks.into_iter().map(|mut block| {
                block.range.start += boundary;
                block.range.end += boundary;
                block
            }));
        self.stable_prefix = self.settled_prefix();
    }

    /// Replacement blocks for the final block while streaming, with its hanging
    /// inline markers closed so styling does not flip as the closer arrives.
    /// `None` means the canonical tree already renders correctly.
    ///
    /// This returns only the tail — never the whole tree — so a streaming
    /// response re-derives O(last block) per delta rather than cloning every
    /// settled block on every frame.
    pub fn display_tail(&self) -> Option<Vec<TopBlock>> {
        let last = self.tree.blocks.last()?;
        // A code block's content is literal: mending would corrupt it, and a
        // half-typed fence must not be reinterpreted.
        if matches!(last.block, Block::CodeBlock { .. }) {
            return None;
        }
        let mended = super::mend::close_hanging(&self.text[last.range.start..])?;
        let offset = last.range.start;
        Some(
            parse(&mended)
                .blocks
                .into_iter()
                .map(|mut block| {
                    block.range.start += offset;
                    block.range.end = (block.range.end + offset).min(self.text.len());
                    block
                })
                .collect(),
        )
    }

    /// The whole tree as it should be displayed. The renderer works from
    /// [`Self::display_tail`] to avoid this clone; this exists so the mending
    /// contract can be asserted directly.

    /// All blocks but the last are settled: markdown block structure only ever
    /// extends the final block, so everything before it is immune to appends.
    fn settled_prefix(&self) -> usize {
        self.tree.blocks.len().saturating_sub(1)
    }
}

/// Cheap scan for a link reference definition (`[label]: destination`), which
/// resolves references anywhere in the document and so breaks locality.
fn has_link_definition(text: &str) -> bool {
    text.lines().any(|line| {
        let line = line.trim_start();
        let Some(rest) = line.strip_prefix('[') else {
            return false;
        };
        rest.find("]:").is_some_and(|end| end > 0)
    })
}
