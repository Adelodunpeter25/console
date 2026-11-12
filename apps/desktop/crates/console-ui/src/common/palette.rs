//! Central command palette modal.
//!
//! [`CommandPaletteModal`] is the shared, app-agnostic shell: a dimmed overlay
//! with one card — a search field, optional section tabs and a scrolling list
//! of rows — drawn from the same pieces as the context menu so it matches the
//! rest of the app. It exposes the small API every palette surface needs:
//! show/hide, replace the entry list, and react to query changes. Feature
//! surfaces (the ⌘K command palette, ⌘P quick file open, ⌘O project browser)
//! are thin wrappers in this crate that feed it [`PaletteEntry`]s and handle
//! confirmation.
//!
//! [`PaletteEntry`] is a plain row: a stable id, a display label, an optional
//! leading icon, and the action to run on Enter. `keep_open` marks entries
//! that navigate (e.g. drilling into a folder) instead of dismissing the
//! palette.

use std::rc::Rc;
use std::sync::Arc;

use gpui::{
    AnyElement, App, Context, ElementId, Entity, FocusHandle, Focusable, FontWeight, Image,
    InteractiveElement, IntoElement, KeyBinding, ParentElement, Render, ScrollHandle,
    SharedString, StatefulInteractiveElement, Styled, Window, div, prelude::*, px,
};

use crate::IconName;
use crate::Theme;
use crate::input::{ComposerEvent, ComposerInput};
use crate::primitives::{
    app_icon, file_type_icon, provider_app_icon, provider_color, provider_color_icon,
};

const CONTEXT: &str = "ConsolePalette";
const FIELD_CONTEXT: &str = "ConsolePalette > ComposerInput";

gpui::actions!(
    console_palette,
    [
        PaletteNext,
        PalettePrev,
        PaletteConfirm,
        PaletteCancel,
        PaletteNextTab,
        PalettePrevTab
    ]
);

/// Bind the palette's keys inside its search field. Called once at startup,
/// after the composer bindings, so these win inside the palette.
pub fn init_palette_keybindings(cx: &mut App) {
    let context = Some(FIELD_CONTEXT);
    cx.bind_keys([
        KeyBinding::new("down", PaletteNext, context),
        KeyBinding::new("up", PalettePrev, context),
        KeyBinding::new("ctrl-n", PaletteNext, context),
        KeyBinding::new("ctrl-p", PalettePrev, context),
        KeyBinding::new("enter", PaletteConfirm, context),
        KeyBinding::new("escape", PaletteCancel, context),
        KeyBinding::new("tab", PaletteNextTab, context),
        KeyBinding::new("shift-tab", PalettePrevTab, context),
    ]);
}

/// How a palette row draws its leading icon.
#[derive(Clone)]
pub enum PaletteIcon {
    /// Monochrome app icon (commands, folders, generic actions).
    App(IconName),
    /// Multicolor file-type icon resolved from a filename or path.
    FileType(SharedString),
    /// Custom image (e.g., website favicon).
    Image(Arc<Image>),
    /// Provider brand mark (e.g. Claude, OpenAI) — same icon/colour chat
    /// tabs use, keyed by provider id (e.g. "claude", "openai").
    Provider(SharedString),
}

/// One palette row: a stable id, display label, and the action to run on Enter.
#[derive(Clone)]
pub struct PaletteEntry {
    pub id: SharedString,
    pub label: SharedString,
    pub icon: Option<PaletteIcon>,
    /// When true the modal stays open after this entry is confirmed (used for
    /// pure navigation rows such as ".." — Enter drills without selecting).
    pub keep_open: bool,
    /// Optional path shown as a trailing chevron. Clicking the chevron drills
    /// into that path (via the browse callback) without confirming the row —
    /// so Enter can mean "use this folder" while the chevron means "open it".
    pub drill_path: Option<SharedString>,
    /// Heading the row is listed under. Rows sharing a heading are grouped;
    /// rows without one stay ungrouped at the top.
    pub section: Option<SharedString>,
    /// Muted text on the row's right edge, such as a last-modified time.
    pub detail: Option<SharedString>,
    /// Keyboard shortcut shown on the row's right edge.
    pub shortcut: Option<SharedString>,
    /// Muted text right after the label, such as a file's directory.
    pub subtitle: Option<SharedString>,
    #[allow(clippy::type_complexity)]
    pub handler: Rc<dyn Fn(&mut Window, &mut App)>,
}

impl PaletteEntry {
    pub fn new(
        id: impl Into<SharedString>,
        label: impl Into<SharedString>,
        handler: impl Fn(&mut Window, &mut App) + 'static,
    ) -> Self {
        Self {
            id: id.into(),
            label: label.into(),
            icon: None,
            keep_open: false,
            drill_path: None,
            section: None,
            detail: None,
            shortcut: None,
            subtitle: None,
            handler: Rc::new(handler),
        }
    }

    /// List the row under a section heading.
    pub fn section(mut self, section: impl Into<SharedString>) -> Self {
        self.section = Some(section.into());
        self
    }

    /// Muted text right after the label.
    pub fn subtitle(mut self, subtitle: impl Into<SharedString>) -> Self {
        self.subtitle = Some(subtitle.into());
        self
    }

    /// Keyboard shortcut for the row's action.
    pub fn shortcut(mut self, shortcut: impl Into<SharedString>) -> Self {
        self.shortcut = Some(shortcut.into());
        self
    }

    /// Muted text on the row's right edge.
    pub fn detail(mut self, detail: impl Into<SharedString>) -> Self {
        self.detail = Some(detail.into());
        self
    }

    /// Leading monochrome app icon (Folder, commands, …).
    pub fn icon(mut self, icon: IconName) -> Self {
        self.icon = Some(PaletteIcon::App(icon));
        self
    }

    /// Leading multicolor file-type icon resolved from a filename or path.
    pub fn file_icon(mut self, path_or_name: impl Into<SharedString>) -> Self {
        self.icon = Some(PaletteIcon::FileType(path_or_name.into()));
        self
    }

    /// Custom image icon (e.g., website favicon).
    pub fn favicon(mut self, image: Arc<Image>) -> Self {
        self.icon = Some(PaletteIcon::Image(image));
        self
    }

    /// Leading provider brand icon — same mark/colour used on chat tabs.
    /// Callers should fall back to `.icon(IconName::ChatRoundLine)` when
    /// there's no known provider.
    pub fn provider_icon(mut self, provider: impl Into<SharedString>) -> Self {
        self.icon = Some(PaletteIcon::Provider(provider.into()));
        self
    }

    /// Keep the palette open after this entry is confirmed. Navigation rows
    /// ("..", …) use this; actions that finish the flow leave it `false`.
    pub fn keep_open(mut self, keep_open: bool) -> Self {
        self.keep_open = keep_open;
        self
    }

    /// Show a trailing chevron that drills into `path` without confirming.
    /// Used by folder rows so Enter = select, chevron/▶ = open.
    pub fn drill_into(mut self, path: impl Into<SharedString>) -> Self {
        self.drill_path = Some(path.into());
        self
    }
}

/// One flattened list row: a section heading or an entry.
enum Row {
    Heading(SharedString),
    Item { entry: usize, visible: usize },
}

/// What the list shows for the current query and tab.
struct Layout {
    rows: Vec<Row>,
    /// Entry indexes in display order — what the selection moves through.
    visible: Vec<usize>,
    /// Row index of each visible entry, for scrolling it into view.
    row_of_visible: Vec<usize>,
    /// Section names, in order of first appearance.
    sections: Vec<SharedString>,
}

/// Rank `text` against a lowercase query: prefix, then substring, then
/// in-order letters. `None` means no match.
fn match_rank(text: &str, query: &str) -> Option<u32> {
    let text = text.to_lowercase();
    if text.starts_with(query) {
        return Some(0);
    }
    if let Some(pos) = text.find(query) {
        return Some(1 + pos as u32);
    }
    let mut letters = text.chars();
    if query.chars().all(|q| letters.any(|c| c == q)) {
        return Some(10_000);
    }
    None
}

fn compute_layout(
    entries: &[PaletteEntry],
    query: &str,
    filterable: bool,
    tab: Option<&SharedString>,
) -> Layout {
    let mut sections: Vec<SharedString> = Vec::new();
    for entry in entries {
        if let Some(section) = &entry.section
            && !sections.contains(section)
        {
            sections.push(section.clone());
        }
    }

    let query = query.trim().to_lowercase();
    let filtering = filterable && !query.is_empty();
    let mut candidates: Vec<(usize, u32)> = Vec::new();
    for (ix, entry) in entries.iter().enumerate() {
        if tab.is_some_and(|tab| entry.section.as_ref() != Some(tab)) {
            continue;
        }
        let rank = if filtering {
            match match_rank(&entry.label, &query).or_else(|| {
                entry
                    .subtitle
                    .as_ref()
                    .and_then(|subtitle| match_rank(subtitle, &query))
                    .map(|rank| rank + 20_000)
            }) {
                Some(rank) => rank,
                None => continue,
            }
        } else {
            0
        };
        candidates.push((ix, rank));
    }
    if filtering {
        candidates.sort_by_key(|&(_, rank)| rank);
    }

    let mut layout = Layout {
        rows: Vec::new(),
        visible: Vec::new(),
        row_of_visible: Vec::new(),
        sections,
    };
    let push_item = |layout: &mut Layout, entry: usize| {
        let visible = layout.visible.len();
        layout.row_of_visible.push(layout.rows.len());
        layout.visible.push(entry);
        layout.rows.push(Row::Item { entry, visible });
    };
    for &(ix, _) in candidates.iter().filter(|&&(ix, _)| entries[ix].section.is_none()) {
        push_item(&mut layout, ix);
    }
    for section in layout.sections.clone() {
        let mut heading_pending = tab.is_none();
        for &(ix, _) in candidates
            .iter()
            .filter(|&&(ix, _)| entries[ix].section.as_ref() == Some(&section))
        {
            if heading_pending {
                layout.rows.push(Row::Heading(section.clone()));
                heading_pending = false;
            }
            push_item(&mut layout, ix);
        }
    }
    layout
}

fn leading_icon(entry: &PaletteEntry, theme: &Theme) -> AnyElement {
    match &entry.icon {
        Some(PaletteIcon::App(name)) => {
            app_icon(*name, 16.0, theme.text_secondary).into_any_element()
        }
        Some(PaletteIcon::FileType(path)) => file_type_icon(path.as_ref(), 16.0).into_any_element(),
        Some(PaletteIcon::Image(image)) => gpui::img(image.clone())
            .h(px(16.0))
            .w(px(16.0))
            .into_any_element(),
        Some(PaletteIcon::Provider(provider)) => {
            match provider_color_icon(provider.as_ref(), 16.0) {
                Some(logo) => logo.into_any_element(),
                None => provider_app_icon(
                    provider.as_ref(),
                    16.0,
                    match provider.as_ref() {
                        "claude" | "anthropic" | "deepseek" => {
                            provider_color(theme, provider.as_ref())
                        }
                        _ => theme.text_secondary,
                    },
                )
                .into_any_element(),
            }
        }
        None => div().size(px(16.0)).into_any_element(),
    }
}

/// One entry row: icon, label, optional subtitle, then the trailing detail,
/// shortcut, drill chevron and — on the selected row — the Enter hint.
fn entry_row(
    entry: &PaletteEntry,
    ix: usize,
    selected: bool,
    theme: &Theme,
    palette: gpui::WeakEntity<CommandPaletteModal>,
) -> AnyElement {
    let has_subtitle = entry.subtitle.is_some();
    let mut row = div()
        .id(("palette-row", ix))
        .w_full()
        .h(px(38.0))
        .px(px(12.0))
        .rounded(px(8.0))
        .flex()
        .items_center()
        .gap(px(10.0))
        .cursor_pointer()
        .when(selected, |row| row.bg(theme.overlay_strong))
        .on_mouse_move({
            let palette = palette.clone();
            move |_, _, cx| {
                if let Some(palette) = palette.upgrade() {
                    palette.update(cx, |palette, cx| {
                        if palette.selected != ix {
                            palette.selected = ix;
                            cx.notify();
                        }
                    });
                }
            }
        })
        .child(
            div()
                .flex_none()
                .size(px(18.0))
                .flex()
                .items_center()
                .justify_center()
                .child(leading_icon(entry, theme)),
        )
        .child(
            div()
                .min_w_0()
                .truncate()
                .text_size(px(13.0))
                .text_color(theme.text)
                .when(!has_subtitle, |label| label.flex_1())
                .when(has_subtitle, |label| label.flex_none().max_w(px(280.0)))
                .child(entry.label.clone()),
        )
        .when_some(entry.subtitle.clone(), |row, subtitle| {
            row.child(
                div()
                    .flex_1()
                    .min_w_0()
                    .truncate()
                    .text_size(px(12.0))
                    .text_color(theme.text_ghost)
                    .child(subtitle),
            )
        })
        .when_some(entry.detail.clone(), |row, detail| {
            row.child(
                div()
                    .flex_none()
                    .text_size(px(12.0))
                    .text_color(theme.text_ghost)
                    .child(detail),
            )
        })
        .when_some(entry.shortcut.clone(), |row, shortcut| {
            row.child(
                div()
                    .flex_none()
                    .text_size(px(12.0))
                    .text_color(theme.text_ghost)
                    .child(shortcut),
            )
        });

    if let Some(drill_path) = entry.drill_path.clone() {
        row = row.child(
            div()
                .id(("palette-drill", ix))
                .flex_none()
                .size(px(24.0))
                .flex()
                .items_center()
                .justify_center()
                .rounded(px(6.0))
                .hover(|chevron| chevron.bg(theme.raised))
                .on_mouse_down(gpui::MouseButton::Left, {
                    let palette = palette.clone();
                    move |_, window, cx| {
                        // Don't let the row's confirm handler fire.
                        cx.stop_propagation();
                        if let Some(palette) = palette.upgrade() {
                            let callback = palette.read(cx).browse_callback.clone();
                            if let Some(callback) = callback {
                                callback(drill_path.as_ref(), window, cx);
                            }
                        }
                    }
                })
                .child(app_icon(IconName::ChevronRight, 14.0, theme.text_tertiary)),
        );
    }
    if selected {
        row = row.child(
            div()
                .flex_none()
                .child(app_icon(IconName::Return, 14.0, theme.text_tertiary)),
        );
    }

    let entry = entry.clone();
    row.on_click(move |_, window, cx| {
        if let Some(palette) = palette.upgrade() {
            palette.update(cx, |palette, cx| {
                palette.confirm_entry(entry.clone(), window, cx)
            });
        }
    })
    .into_any_element()
}

/// The shared command palette modal shell.
///
/// Renders nothing while closed. The owning wrapper supplies entries via
/// [`Self::set_entries`] (async results call it again — it notifies) and, for
/// search-driven modes, a query handler via [`Self::set_query_handler`] that
/// runs with the trimmed query whenever it changes.
pub struct CommandPaletteModal {
    input: Entity<ComposerInput>,
    entries: Rc<Vec<PaletteEntry>>,
    open: bool,
    /// Whether the query locally filters the item list. Set to `false` when an
    /// external source already answers the query (e.g. server-side file search).
    filterable: bool,
    /// Index into the visible entries.
    selected: usize,
    /// The section tab in front; `None` is "All".
    tab: Option<SharedString>,
    scroll: ScrollHandle,
    /// Async search hook: called with the current query whenever it changes.
    query_handler: Option<Rc<dyn Fn(&str, &mut Window, &mut App)>>,
    /// Navigation hook for `keep_open` rows (e.g. drilling into a folder):
    /// called with the entry's id instead of dismissing the palette.
    browse_callback: Option<Rc<dyn Fn(&str, &mut Window, &mut App)>>,
    /// Monotonic token for debounced async searches: a result is applied only
    /// while its generation is still the latest (see [`Self::set_entries`]).
    search_generation: u64,
    _subscription: gpui::Subscription,
}

impl CommandPaletteModal {
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let input = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .search_field()
                .placeholder("Type a command or search…")
        });
        let subscription = cx.subscribe_in(
            &input,
            window,
            |this, input, event: &ComposerEvent, window, cx| {
                if matches!(event, ComposerEvent::Edited) {
                    this.selected = 0;
                    this.scroll.set_offset(gpui::point(px(0.0), px(0.0)));
                    let query = input.read(cx).content().trim().to_string();
                    if let Some(handler) = this.query_handler.clone() {
                        handler(&query, window, cx);
                    }
                    cx.notify();
                }
            },
        );
        Self {
            input,
            entries: Rc::new(Vec::new()),
            open: false,
            filterable: true,
            selected: 0,
            tab: None,
            scroll: ScrollHandle::new(),
            query_handler: None,
            browse_callback: None,
            search_generation: 0,
            _subscription: subscription,
        }
    }

    fn query(&self, cx: &App) -> String {
        self.input.read(cx).content().to_string()
    }

    fn layout(&self, cx: &App) -> Layout {
        compute_layout(
            &self.entries,
            &self.query(cx),
            self.filterable,
            self.tab.as_ref(),
        )
    }

    /// Reset query, tab and selection to a fresh empty palette.
    pub fn reset_state(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        self.input.update(cx, |input, cx| input.clear(cx));
        self.selected = 0;
        self.tab = None;
        self.search_generation = 0;
        self.scroll.set_offset(gpui::point(px(0.0), px(0.0)));
        cx.notify();
    }

    /// Clear the search query so navigation keeps focus.
    pub fn clear_query(&mut self, _window: &mut Window, cx: &mut Context<Self>) {
        self.input.update(cx, |input, cx| input.clear(cx));
        self.selected = 0;
        cx.notify();
    }

    /// Replace the entry list. Safe to call from async contexts — notifies so
    /// the modal re-renders with the fresh rows.
    pub fn set_entries(&mut self, entries: Vec<PaletteEntry>, cx: &mut Context<Self>) {
        self.entries = Rc::new(entries);
        if let Some(tab) = &self.tab
            && !self
                .entries
                .iter()
                .any(|entry| entry.section.as_ref() == Some(tab))
        {
            self.tab = None;
        }
        cx.notify();
    }

    /// Drop the generation token so a fresh open can apply its first result.
    pub fn reset_search_generation(&mut self, cx: &mut Context<Self>) {
        self.search_generation = 0;
        cx.notify();
    }

    /// [`Self::set_entries`] guarded by a search generation token.
    pub fn set_entries_with_generation(
        &mut self,
        generation: u64,
        entries: Vec<PaletteEntry>,
        cx: &mut Context<Self>,
    ) {
        if generation != self.search_generation {
            return;
        }
        self.set_entries(entries, cx);
    }

    /// Set the placeholder shown in the search field.
    pub fn set_placeholder(
        &mut self,
        placeholder: impl Into<SharedString>,
        cx: &mut Context<Self>,
    ) {
        let placeholder = placeholder.into();
        self.input
            .update(cx, |input, cx| input.set_placeholder(placeholder, cx));
    }

    /// Turn local query filtering on or off. Defaults to `true`; turn it off
    /// when an external source already answers the query.
    pub fn set_filterable(&mut self, filterable: bool, cx: &mut Context<Self>) {
        self.filterable = filterable;
        cx.notify();
    }

    /// Register the callback run whenever the search query changes. Replace it
    /// on every open so it captures the latest context.
    pub fn set_query_handler(
        &mut self,
        handler: impl Fn(&str, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.query_handler = Some(Rc::new(handler));
    }

    /// Register the navigation hook for `keep_open` rows. It receives the
    /// confirmed entry's id and keeps the modal open (e.g. browsing into a
    /// folder). Replaced on every open.
    pub fn set_browse_callback(
        &mut self,
        callback: impl Fn(&str, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.browse_callback = Some(Rc::new(callback));
    }

    pub fn show(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.open = true;
        let focus = self.input.read(cx).focus();
        window.focus(&focus, cx);
        cx.notify();
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.open = false;
        cx.notify();
    }

    pub fn toggle(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        if self.open {
            self.hide(cx);
        } else {
            self.show(window, cx);
        }
    }

    pub fn is_open(&self) -> bool {
        self.open
    }

    /// Run an entry. Its action runs after the current update finishes: the
    /// action may open or toggle palettes, including this one.
    fn confirm_entry(&mut self, entry: PaletteEntry, window: &mut Window, cx: &mut Context<Self>) {
        if entry.keep_open {
            // Navigation row: hand the id to the browse hook and keep the
            // palette up.
            if let Some(callback) = self.browse_callback.clone() {
                let id = entry.id.clone();
                window.defer(cx, move |window, cx| callback(id.as_ref(), window, cx));
            }
        } else {
            self.hide(cx);
            let handler = entry.handler.clone();
            window.defer(cx, move |window, cx| handler(window, cx));
        }
    }

    fn move_selection(&mut self, delta: isize, cx: &mut Context<Self>) {
        let layout = self.layout(cx);
        let len = layout.visible.len() as isize;
        if len == 0 {
            return;
        }
        let current = self.selected.min(layout.visible.len() - 1) as isize;
        self.selected = (current + delta).rem_euclid(len) as usize;
        if let Some(&row) = layout.row_of_visible.get(self.selected) {
            self.scroll.scroll_to_item(row);
        }
        cx.notify();
    }

    fn step_tab(&mut self, delta: isize, cx: &mut Context<Self>) {
        let layout = self.layout(cx);
        if layout.sections.len() < 2 {
            return;
        }
        // Position 0 is "All"; sections follow.
        let current = self
            .tab
            .as_ref()
            .and_then(|tab| layout.sections.iter().position(|section| section == tab))
            .map_or(0, |ix| ix + 1) as isize;
        let next = (current + delta).rem_euclid(layout.sections.len() as isize + 1) as usize;
        self.tab = next.checked_sub(1).map(|ix| layout.sections[ix].clone());
        self.selected = 0;
        self.scroll.set_offset(gpui::point(px(0.0), px(0.0)));
        cx.notify();
    }

    fn on_confirm(&mut self, _: &PaletteConfirm, window: &mut Window, cx: &mut Context<Self>) {
        let layout = self.layout(cx);
        let Some(&entry_ix) = layout
            .visible
            .get(self.selected.min(layout.visible.len().saturating_sub(1)))
        else {
            return;
        };
        if let Some(entry) = self.entries.get(entry_ix).cloned() {
            self.confirm_entry(entry, window, cx);
        }
    }

    /// Escape clears a non-empty query first; once it is empty the palette
    /// closes.
    fn on_cancel(&mut self, _: &PaletteCancel, window: &mut Window, cx: &mut Context<Self>) {
        if self.query(cx).is_empty() {
            self.hide(cx);
        } else {
            self.clear_query(window, cx);
        }
    }
}

impl Focusable for CommandPaletteModal {
    fn focus_handle(&self, cx: &App) -> FocusHandle {
        self.input.read(cx).focus()
    }
}

impl Render for CommandPaletteModal {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        if !self.open {
            return div().into_any_element();
        }

        let theme = Theme::current(cx);
        let layout = self.layout(cx);
        let selected = self.selected.min(layout.visible.len().saturating_sub(1));
        let palette = cx.entity().downgrade();
        let has_query = !self.query(cx).trim().is_empty();

        let mut list = div()
            .id("palette-list")
            .flex_1()
            .min_h_0()
            .max_h(px(380.0))
            .overflow_y_scroll()
            .track_scroll(&self.scroll)
            .p(px(6.0))
            .flex()
            .flex_col()
            .gap(px(2.0));
        for row in &layout.rows {
            list = list.child(match row {
                Row::Heading(label) => div()
                    .px(px(12.0))
                    .pt(px(14.0))
                    .pb(px(6.0))
                    .text_size(px(11.5))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(theme.text_tertiary)
                    .child(label.clone())
                    .into_any_element(),
                Row::Item { entry, visible } => entry_row(
                    &self.entries[*entry],
                    *visible,
                    *visible == selected,
                    &theme,
                    palette.clone(),
                ),
            });
        }

        let tabs = (layout.sections.len() >= 2).then(|| {
            let mut bar = div()
                .flex_none()
                .px(px(8.0))
                .pt(px(6.0))
                .pb(px(2.0))
                .flex()
                .items_center()
                .gap(px(2.0));
            let labels: Vec<Option<SharedString>> = std::iter::once(None)
                .chain(layout.sections.iter().cloned().map(Some))
                .collect();
            for (ix, label) in labels.into_iter().enumerate() {
                let active = label == self.tab;
                let text: SharedString = label.clone().unwrap_or_else(|| "All".into());
                let palette = palette.clone();
                bar = bar.child(
                    div()
                        .id(ElementId::from(("palette-tab", ix)))
                        .h(px(24.0))
                        .px(px(9.0))
                        .rounded(px(6.0))
                        .flex()
                        .items_center()
                        .text_size(px(12.0))
                        .cursor_pointer()
                        .when(active, |tab| {
                            tab.bg(theme.overlay_strong).text_color(theme.text)
                        })
                        .when(!active, |tab| {
                            tab.text_color(theme.text_tertiary)
                                .hover(|tab| tab.bg(theme.overlay))
                        })
                        .on_click(move |_, _, cx| {
                            if let Some(palette) = palette.upgrade() {
                                palette.update(cx, |palette, cx| {
                                    palette.tab = label.clone();
                                    palette.selected = 0;
                                    cx.notify();
                                });
                            }
                        })
                        .child(text),
                );
            }
            bar
        });

        let empty = layout.rows.is_empty() && has_query;
        let card = div()
            .occlude()
            .key_context(CONTEXT)
            .on_action(cx.listener(Self::on_confirm))
            .on_action(cx.listener(Self::on_cancel))
            .on_action(cx.listener(|this, _: &PaletteNext, _, cx| this.move_selection(1, cx)))
            .on_action(cx.listener(|this, _: &PalettePrev, _, cx| this.move_selection(-1, cx)))
            .on_action(cx.listener(|this, _: &PaletteNextTab, _, cx| this.step_tab(1, cx)))
            .on_action(cx.listener(|this, _: &PalettePrevTab, _, cx| this.step_tab(-1, cx)))
            .on_mouse_down(gpui::MouseButton::Left, |_, _, cx| cx.stop_propagation())
            .w(px(560.0))
            .max_h(px(480.0))
            .flex()
            .flex_col()
            .overflow_hidden()
            .rounded(px(10.0))
            .border_1()
            .border_color(theme.border_strong)
            .bg(theme.menu)
            .shadow_lg()
            .child(
                div()
                    .flex_none()
                    .h(px(46.0))
                    .px(px(14.0))
                    .flex()
                    .items_center()
                    .gap(px(12.0))
                    .border_b_1()
                    .border_color(theme.border)
                    .text_size(px(14.0))
                    .child(
                        div()
                            .flex_none()
                            .child(app_icon(IconName::Search, 15.0, theme.text_tertiary)),
                    )
                    .child(div().flex_1().min_w_0().child(self.input.clone())),
            )
            .children(tabs)
            .when(!layout.rows.is_empty(), |card| card.child(list))
            .when(empty, |card| {
                card.child(
                    div()
                        .py(px(28.0))
                        .flex()
                        .justify_center()
                        .text_size(px(12.5))
                        .text_color(theme.text_tertiary)
                        .child("No results"),
                )
            });

        // Modal overlay: dimmed backdrop, palette centered near the top.
        // Painted last inside app-root, so no explicit z-index is needed.
        div()
            .absolute()
            .inset_0()
            .bg(theme.backdrop)
            .flex()
            .justify_center()
            .items_start()
            .pt(px(96.0))
            .on_mouse_down(gpui::MouseButton::Left, {
                let entity = cx.entity().downgrade();
                move |_, _, cx| {
                    if let Some(palette) = entity.upgrade() {
                        palette.update(cx, |palette, cx| palette.hide(cx));
                    }
                }
            })
            .child(card)
            .into_any_element()
    }
}

/// ⌘K command palette surface: static entries for app commands, built on the
/// shared [`CommandPaletteModal`].
pub struct CommandPalette {
    modal: Entity<CommandPaletteModal>,
}

impl CommandPalette {
    /// Create the palette. Reuse one instance per surface; entries can be
    /// replaced any time via [`Self::set_entries`].
    pub fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let modal = cx.new(|cx| CommandPaletteModal::new(window, cx));
        Self { modal }
    }

    /// Replace the entry list. Call before `show()` so filtering and
    /// IndexPath addressing match.
    pub fn set_entries(&mut self, entries: Vec<PaletteEntry>, cx: &mut Context<Self>) {
        self.modal
            .update(cx, |modal, cx| modal.set_entries(entries, cx));
    }

    /// Update the search-field placeholder text.
    pub fn set_placeholder(
        &mut self,
        placeholder: impl Into<SharedString>,
        cx: &mut Context<Self>,
    ) {
        self.modal
            .update(cx, |modal, cx| modal.set_placeholder(placeholder, cx));
    }

    /// Run `handler` with the trimmed query whenever it changes. Replace it on
    /// every open so it captures the latest context.
    pub fn set_query_handler(
        &mut self,
        handler: impl Fn(&str, &mut Window, &mut App) + 'static,
        cx: &mut Context<Self>,
    ) {
        self.modal
            .update(cx, |modal, cx| modal.set_query_handler(handler, cx));
    }

    pub fn show(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| {
            modal.reset_state(window, cx);
            modal.show(window, cx);
        });
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.hide(cx));
    }

    pub fn toggle(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| {
            if modal.is_open() {
                modal.hide(cx);
            } else {
                modal.reset_state(window, cx);
                modal.show(window, cx);
            }
        });
    }

    pub fn is_open(&self, cx: &App) -> bool {
        self.modal.read(cx).is_open()
    }
}

impl Render for CommandPalette {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        self.modal.clone()
    }
}
