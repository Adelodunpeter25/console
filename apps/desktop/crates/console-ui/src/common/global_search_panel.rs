//! ⌘⇧F global content search panel.
//!
//! A hand-rolled modal (not built on `gpui_component::command`): the search
//! field needs an inline toggle row that `Command`'s single query field
//! doesn't expose, and results are grouped by file with per-match highlight
//! ranges rather than flat filterable rows — both fit a custom shell better
//! than bending the shared command palette.
//!
//! v1 ships a plain search box (no case/whole-word/regex toggles yet) with
//! results grouped under their file name and the match substring highlighted
//! from the server's byte-offset `matchRanges`.

use std::rc::Rc;

use console_core::{ConsoleClient, GrepMatch, GrepOptions};
use gpui::{
    App, AppContext, Context, Entity, FocusHandle, Focusable, InteractiveElement, IntoElement,
    KeyBinding, ParentElement, Render, SharedString, Styled, Window, div, prelude::FluentBuilder,
    px, uniform_list,
};
use gpui_component::input::{Input, InputEvent, InputState};

use crate::Theme;
use crate::primitives::{app_icon, file_type_icon};
use crate::IconName;

/// Debounce for typed queries.
const SEARCH_DEBOUNCE_MS: u64 = 150;
/// Fixed row height so `uniform_list` can virtualize without measuring each
/// row. File-heading rows and match rows share this height (the heading row
/// simply has more vertical padding around shorter text).
const ROW_HEIGHT_PX: f32 = 26.0;

const CONTEXT: &str = "GlobalSearchPanel";

gpui::actions!(global_search_panel, [Cancel, ConfirmMatch, SelectNext, SelectPrev]);

pub(crate) fn init(cx: &mut App) {
    let context: Option<&str> = Some(CONTEXT);
    cx.bind_keys([
        KeyBinding::new("escape", Cancel, context),
        KeyBinding::new("enter", ConfirmMatch, context),
        KeyBinding::new("down", SelectNext, context),
        KeyBinding::new("up", SelectPrev, context),
    ]);
}

/// One file's matches, grouped for rendering.
struct FileGroup {
    rel_path: String,
    file_name: String,
    matches: Vec<GrepMatch>,
}

fn group_matches(matches: Vec<GrepMatch>) -> Vec<FileGroup> {
    let mut groups: Vec<FileGroup> = Vec::new();
    for m in matches {
        if let Some(last) = groups.last_mut() {
            if last.rel_path == m.rel_path {
                last.matches.push(m);
                continue;
            }
        }
        groups.push(FileGroup {
            rel_path: m.rel_path.clone(),
            file_name: m.file_name.clone(),
            matches: vec![m],
        });
    }
    groups
}

/// A single flattened row: either a file heading or a match line, so the
/// keyboard selection can move through them uniformly.
enum SearchRow {
    Heading { group_ix: usize },
    Match { group_ix: usize, match_ix: usize },
}

pub struct GlobalSearchPanel {
    client: ConsoleClient,
    query_input: Entity<InputState>,
    focus_handle: FocusHandle,
    open: bool,
    root: Option<String>,
    groups: Rc<Vec<FileGroup>>,
    rows: Rc<Vec<SearchRow>>,
    selected_row: Option<usize>,
    total_matched: usize,
    matched_files: usize,
    files_searched: usize,
    has_searched: bool,
    loading: bool,
    search_generation: u64,
    on_open_match: Option<Rc<dyn Fn(&str, u64, &mut Window, &mut App)>>,
    _subscriptions: Vec<gpui::Subscription>,
}

impl GlobalSearchPanel {
    pub fn new(client: ConsoleClient, window: &mut Window, cx: &mut Context<Self>) -> Self {
        init(cx);
        let query_input = cx.new(|cx| InputState::new(window, cx));
        let subscription = cx.subscribe_in(&query_input, window, Self::on_query_input_event);
        Self {
            client,
            query_input,
            focus_handle: cx.focus_handle(),
            open: false,
            root: None,
            groups: Rc::new(Vec::new()),
            rows: Rc::new(Vec::new()),
            selected_row: None,
            total_matched: 0,
            matched_files: 0,
            files_searched: 0,
            has_searched: false,
            loading: false,
            search_generation: 0,
            on_open_match: None,
            _subscriptions: vec![subscription],
        }
    }

    /// Callback invoked when a match row is confirmed: absolute-ish path
    /// (joined with root by the caller if needed) and 1-based line number.
    pub fn set_on_open_match(
        &mut self,
        callback: impl Fn(&str, u64, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_open_match = Some(Rc::new(callback));
    }

    pub fn open(&mut self, root: Option<String>, window: &mut Window, cx: &mut Context<Self>) {
        self.root = root;
        self.open = true;
        self.groups = Rc::new(Vec::new());
        self.rows = Rc::new(Vec::new());
        self.selected_row = None;
        self.total_matched = 0;
        self.matched_files = 0;
        self.files_searched = 0;
        self.has_searched = false;
        self.loading = false;
        self.search_generation += 1;
        self.query_input
            .update(cx, |input, cx| input.set_value("", window, cx));
        window.focus(&self.query_input.focus_handle(cx), cx);
        cx.notify();
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.open = false;
        cx.notify();
    }

    pub fn is_open(&self) -> bool {
        self.open
    }

    fn on_query_input_event(
        &mut self,
        _input: &Entity<InputState>,
        event: &InputEvent,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if !matches!(event, InputEvent::Change) {
            return;
        }
        let query = self.query_input.read(cx).value().to_string();
        self.schedule_search(query, window, cx);
    }

    fn schedule_search(&mut self, query: String, _window: &mut Window, cx: &mut Context<Self>) {
        self.search_generation += 1;
        let generation = self.search_generation;

        if query.trim().is_empty() {
            self.groups = Rc::new(Vec::new());
            self.rows = Rc::new(Vec::new());
            self.selected_row = None;
            self.total_matched = 0;
            self.files_searched = 0;
            self.has_searched = false;
            self.loading = false;
            cx.notify();
            return;
        }

        let Some(root) = self.root.clone() else {
            return;
        };
        self.loading = true;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |this, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_millis(SEARCH_DEBOUNCE_MS))
                .await;

            let still_current = this
                .read_with(cx, |this, _| this.search_generation == generation)
                .unwrap_or(false);
            if !still_current {
                return;
            }

            let options = GrepOptions::default();
            let result = client.fs.grep(&root, &query, &options).await;

            let _ = this.update(cx, |this, cx| {
                if generation != this.search_generation {
                    return;
                }
                this.loading = false;
                this.has_searched = true;
                match result {
                    Ok(res) => {
                        this.total_matched = res.total_matched;
                        this.files_searched = res.files_searched;
                        let groups = group_matches(res.matches);
                        this.matched_files = if res.filtered_files > 0 {
                            res.filtered_files
                        } else {
                            groups.len()
                        };
                        let mut rows = Vec::new();
                        for (group_ix, group) in groups.iter().enumerate() {
                            rows.push(SearchRow::Heading { group_ix });
                            for match_ix in 0..group.matches.len() {
                                rows.push(SearchRow::Match {
                                    group_ix,
                                    match_ix,
                                });
                            }
                        }
                        this.selected_row = rows
                            .iter()
                            .position(|row| matches!(row, SearchRow::Match { .. }));
                        this.groups = Rc::new(groups);
                        this.rows = Rc::new(rows);
                    }
                    Err(_) => {
                        this.total_matched = 0;
                        this.matched_files = 0;
                        this.files_searched = 0;
                        this.groups = Rc::new(Vec::new());
                        this.rows = Rc::new(Vec::new());
                        this.selected_row = None;
                    }
                }
                cx.notify();
            });
        })
        .detach();
    }

    fn confirm_row(&mut self, row_ix: usize, window: &mut Window, cx: &mut Context<Self>) {
        let Some(SearchRow::Match {
            group_ix,
            match_ix,
        }) = self.rows.get(row_ix)
        else {
            return;
        };
        let Some(group) = self.groups.get(*group_ix) else {
            return;
        };
        let Some(m) = group.matches.get(*match_ix) else {
            return;
        };
        let rel_path = m.rel_path.clone();
        let line_number = m.line_number;
        let callback = self.on_open_match.clone();
        let window_handle = window.window_handle();

        // Hide synchronously, but defer opening the file until this panel update
        // has returned. Opening a tab updates the parent app immediately, and
        // that update reads this panel to detect open palettes; invoking the
        // callback inline makes GPUI read the panel while it is still being
        // updated, which aborts the process.
        self.hide(cx);
        if let Some(callback) = callback {
            cx.defer(move |cx| {
                let _ = window_handle.update(cx, |_, window, cx| {
                    callback(&rel_path, line_number, window, cx);
                });
            });
        }
    }

    fn move_selection(&mut self, delta: isize, cx: &mut Context<Self>) {
        let match_row_ixs: Vec<usize> = self
            .rows
            .iter()
            .enumerate()
            .filter(|(_, row)| matches!(row, SearchRow::Match { .. }))
            .map(|(ix, _)| ix)
            .collect();
        if match_row_ixs.is_empty() {
            return;
        }
        let current_pos = self
            .selected_row
            .and_then(|sel| match_row_ixs.iter().position(|&ix| ix == sel))
            .unwrap_or(0);
        let len = match_row_ixs.len() as isize;
        let next_pos = (current_pos as isize + delta).rem_euclid(len) as usize;
        self.selected_row = Some(match_row_ixs[next_pos]);
        cx.notify();
    }

    fn on_action_cancel(&mut self, _: &Cancel, _window: &mut Window, cx: &mut Context<Self>) {
        self.hide(cx);
    }

    fn on_action_confirm(&mut self, _: &ConfirmMatch, window: &mut Window, cx: &mut Context<Self>) {
        if let Some(row_ix) = self.selected_row {
            self.confirm_row(row_ix, window, cx);
        }
    }

    fn on_action_next(&mut self, _: &SelectNext, _window: &mut Window, cx: &mut Context<Self>) {
        self.move_selection(1, cx);
    }

    fn on_action_prev(&mut self, _: &SelectPrev, _window: &mut Window, cx: &mut Context<Self>) {
        self.move_selection(-1, cx);
    }
}

impl Focusable for GlobalSearchPanel {
    fn focus_handle(&self, cx: &App) -> FocusHandle {
        self.query_input.focus_handle(cx)
    }
}

/// Render one match's line content with the byte-range `matchRanges` bolded.
fn render_highlighted_line(m: &GrepMatch, theme: &Theme) -> gpui::AnyElement {
    let content = &m.line_content;
    if m.match_ranges.is_empty() {
        return div()
            .text_size(px(12.0))
            .text_color(theme.text_secondary)
            .child(content.clone())
            .into_any_element();
    }

    let mut segments: Vec<gpui::AnyElement> = Vec::new();
    let mut cursor = 0usize;
    for range in &m.match_ranges {
        let start = range.start.min(content.len());
        let end = range.end.min(content.len());
        if start < cursor || start > end {
            continue;
        }
        if start > cursor {
            if let Some(text) = content.get(cursor..start) {
                segments.push(
                    div()
                        .text_color(theme.text_secondary)
                        .child(text.to_string())
                        .into_any_element(),
                );
            }
        }
        if let Some(text) = content.get(start..end) {
            segments.push(
                div()
                    .text_color(theme.text)
                    .font_weight(gpui::FontWeight::BOLD)
                    .bg(theme.accent.opacity(0.18))
                    .child(text.to_string())
                    .into_any_element(),
            );
        }
        cursor = end;
    }
    if cursor < content.len() {
        if let Some(text) = content.get(cursor..) {
            segments.push(
                div()
                    .text_color(theme.text_secondary)
                    .child(text.to_string())
                    .into_any_element(),
            );
        }
    }

    div()
        .flex()
        .flex_row()
        .flex_wrap()
        .text_size(px(12.0))
        .font_family("monospace")
        .children(segments)
        .into_any_element()
}

impl Render for GlobalSearchPanel {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if !self.open {
            return div().into_any_element();
        }

        let theme = Theme::current(cx);
        let groups = self.groups.clone();
        let rows = self.rows.clone();
        let selected_row = self.selected_row;
        let loading = self.loading;
        let has_searched = self.has_searched;
        let total_matched = self.total_matched;
        let matched_files = self.matched_files;
        let row_count = rows.len();

        let empty_state: Option<gpui::AnyElement> = if !has_searched && !loading {
            Some(
                div()
                    .flex()
                    .flex_col()
                    .items_center()
                    .justify_center()
                    .py(px(48.0))
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("Search across every file in the project")
                    .into_any_element(),
            )
        } else if has_searched && !loading && rows.is_empty() {
            Some(
                div()
                    .flex()
                    .flex_col()
                    .items_center()
                    .justify_center()
                    .py(px(48.0))
                    .text_size(px(12.0))
                    .text_color(theme.text_tertiary)
                    .child("No matches")
                    .into_any_element(),
            )
        } else {
            None
        };

        let header_text: Option<SharedString> = if has_searched && !loading && !rows.is_empty() {
            Some(
                format!(
                    "{} match{} in {} file{}",
                    total_matched,
                    if total_matched == 1 { "" } else { "es" },
                    matched_files,
                    if matched_files == 1 { "" } else { "s" }
                )
                .into(),
            )
        } else {
            None
        };

        let entity = cx.entity().downgrade();
        // Virtualized results: only the visible ~20 rows are ever built into
        // elements, so a 200-match response stays as cheap to scroll as a
        // 5-match one. Row content is looked up by index from `groups`/`rows`
        // inside the closure, which `uniform_list` calls per visible range.
        let results_list = uniform_list(
            "global-search-results",
            row_count,
            move |range, _window, cx| {
                let theme = Theme::current(cx);
                range
                    .map(|row_ix| {
                        render_search_row(row_ix, &rows, &groups, selected_row, &theme, &entity)
                    })
                    .collect::<Vec<_>>()
            },
        )
        .flex_1()
        .py(px(4.0));

        div()
            .absolute()
            .inset_0()
            .bg(gpui::black().opacity(0.35))
            .flex()
            .justify_center()
            .items_start()
            .pt(px(96.0))
            .on_mouse_down(gpui::MouseButton::Left, {
                let entity = cx.entity().downgrade();
                move |_, _, cx| {
                    if let Some(panel) = entity.upgrade() {
                        panel.update(cx, |panel, cx| panel.hide(cx));
                    }
                }
            })
            .child(
                div()
                    .occlude()
                    .key_context(CONTEXT)
                    .track_focus(&self.focus_handle)
                    .on_action(cx.listener(Self::on_action_cancel))
                    .on_action(cx.listener(Self::on_action_confirm))
                    .on_action(cx.listener(Self::on_action_next))
                    .on_action(cx.listener(Self::on_action_prev))
                    .on_mouse_down(gpui::MouseButton::Left, |_, _, cx| {
                        cx.stop_propagation();
                    })
                    .w(px(640.0))
                    .h(px(520.0))
                    .flex()
                    .flex_col()
                    .rounded(theme_radius(&theme))
                    .bg(theme.surface)
                    .border_1()
                    .border_color(theme.border)
                    .overflow_hidden()
                    .child(
                        div()
                            .flex_none()
                            .px(px(12.0))
                            .border_b_1()
                            .border_color(theme.border)
                            .child(
                                Input::new(&self.query_input)
                                    .prefix(app_icon(IconName::Search, 14.0, theme.text_tertiary))
                                    .appearance(false)
                                    .p_0(),
                            ),
                    )
                    .when_some(header_text, |el, text| {
                        el.child(
                            div()
                                .flex_none()
                                .px(px(12.0))
                                .py(px(6.0))
                                .border_b_1()
                                .border_color(theme.border)
                                .text_size(px(11.0))
                                .text_color(theme.text_tertiary)
                                .child(text),
                        )
                    })
                    .when_some(empty_state, |el, empty| el.child(empty))
                    .when(has_searched && !loading && row_count > 0, |el| {
                        el.child(results_list)
                    }),
            )
            .into_any_element()
    }
}

/// Render one virtualized row (file heading or match line) by absolute row
/// index. Called only for the currently visible range by `uniform_list`.
fn render_search_row(
    row_ix: usize,
    rows: &[SearchRow],
    groups: &[FileGroup],
    selected_row: Option<usize>,
    theme: &Theme,
    entity: &gpui::WeakEntity<GlobalSearchPanel>,
) -> gpui::AnyElement {
    let Some(row) = rows.get(row_ix) else {
        return div().h(px(ROW_HEIGHT_PX)).into_any_element();
    };
    match row {
        SearchRow::Heading { group_ix } => {
            let Some(group) = groups.get(*group_ix) else {
                return div().h(px(ROW_HEIGHT_PX)).into_any_element();
            };
            div()
                .h(px(ROW_HEIGHT_PX))
                .flex()
                .flex_row()
                .items_center()
                .gap(px(6.0))
                .px(px(10.0))
                .child(file_type_icon(&group.rel_path, 13.0))
                .child(
                    div()
                        .text_size(px(12.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.text)
                        .child(group.file_name.clone()),
                )
                .child(
                    div()
                        .text_size(px(11.0))
                        .text_color(theme.text_tertiary)
                        .truncate()
                        .child(group.rel_path.clone()),
                )
                .into_any_element()
        }
        SearchRow::Match {
            group_ix,
            match_ix,
        } => {
            let Some(m) = groups
                .get(*group_ix)
                .and_then(|group| group.matches.get(*match_ix))
            else {
                return div().h(px(ROW_HEIGHT_PX)).into_any_element();
            };
            let selected = selected_row == Some(row_ix);
            let entity = entity.clone();
            div()
                .id(("global-search-match", row_ix))
                .h(px(ROW_HEIGHT_PX))
                .flex()
                .flex_row()
                .items_center()
                .gap(px(8.0))
                .px(px(14.0))
                .cursor_pointer()
                .when(selected, |el| el.bg(theme.raised))
                .hover(|el| el.bg(theme.raised))
                .on_mouse_down(gpui::MouseButton::Left, move |_, window, cx| {
                    if let Some(panel) = entity.upgrade() {
                        panel.update(cx, |panel, cx| {
                            panel.confirm_row(row_ix, window, cx);
                        });
                    }
                })
                .child(
                    div()
                        .flex_none()
                        .w(px(32.0))
                        .text_size(px(11.0))
                        .text_color(theme.text_tertiary)
                        .child(m.line_number.to_string()),
                )
                .child(
                    div()
                        .flex_1()
                        .min_w(px(0.0))
                        .overflow_hidden()
                        .child(render_highlighted_line(m, theme)),
                )
                .into_any_element()
        }
    }
}

fn theme_radius(_theme: &Theme) -> gpui::Pixels {
    px(10.0)
}
