//! "New project" dialog: the two ways a project comes into existence from
//! scratch — a blank folder with `git init`, or a clone of a GitHub repo.
//!
//! A separate window rather than an in-window overlay, matching the Settings
//! window. The server call is synchronous, so the window has to stay alive
//! and interactive across a clone that can take minutes; a modal overlay
//! would have to be re-rendered on every busy tick for no benefit.
//!
//! Reachable only from File ▸ New Project…. The existing Add Project flow
//! (⌘O) registers a folder that already exists — a different job from
//! creating one, so neither subsumes the other.

use crate::state::ConsoleDesktopApp;
use console_core::is_github_not_connected;
use console_core::types::FsTreeEntry;
use console_ui::input::ComposerInput;
use console_ui::theme::Theme;
use gpui::{
    App, AppContext, Context, Entity, FocusHandle, Focusable, InteractiveElement, IntoElement,
    KeyDownEvent, MouseButton, ParentElement, Render, StatefulInteractiveElement, Styled, Window,
    div, px,
};
use std::rc::Rc;

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum NewProjectMode {
    Blank,
    Clone,
}

/// Whether a failure needs a different response than "show the message".
///
/// The server answers a private-repo clone with no stored token as 409 rather
/// than 400, precisely so this case can send the user to Accounts instead of
/// implying they typed the URL wrong.
#[derive(Clone, Copy, PartialEq, Eq)]
enum FailureKind {
    Message,
    NotConnected,
}

pub struct NewProjectDialog {
    app: gpui::WeakEntity<ConsoleDesktopApp>,
    focus_handle: FocusHandle,
    mode: NewProjectMode,
    name_input: Entity<ComposerInput>,
    url_input: Entity<ComposerInput>,
    /// The folder the project will be created in — a path on the SERVER's
    /// filesystem, chosen from the browser below. Empty until the user picks
    /// one. Never a locally-selected path: the backend may be a remote host
    /// where the Mac's folders do not exist.
    parent_dir: String,
    /// Folder the browser is currently listing. None until the first response
    /// lands, at which point it is the server's home directory.
    folder_path: Option<String>,
    /// Parent of `folder_path`, rendered as the ".." row.
    folder_parent: Option<String>,
    /// Folders only — files are filtered out of the listing, since this picks
    /// a directory and nothing else.
    folder_entries: Vec<FsTreeEntry>,
    /// Row highlighted by a single click. "Use this folder" commits this.
    folder_selected: Option<String>,
    folder_loading: bool,
    /// Set while the server call is in flight. The call is synchronous, so
    /// this is the whole progress story: inputs and buttons go inert.
    busy: bool,
    error: Option<(FailureKind, String)>,
    _subscriptions: Vec<gpui::Subscription>,
}

impl NewProjectDialog {
    pub fn new(
        app: gpui::WeakEntity<ConsoleDesktopApp>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let subscription = app
            .upgrade()
            .map(|app_entity| cx.observe(&app_entity, |_this, _app, cx| cx.notify()));

        let name_input = cx.new(|cx| ComposerInput::new(window, cx).placeholder("my-project"));
        let url_input =
            cx.new(|cx| ComposerInput::new(window, cx).placeholder("https://github.com/org/repo"));

        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle, cx);

        let mut dialog = Self {
            app,
            focus_handle,
            mode: NewProjectMode::Blank,
            name_input,
            url_input,
            parent_dir: String::new(),
            folder_path: None,
            folder_parent: None,
            folder_entries: Vec::new(),
            folder_selected: None,
            folder_loading: false,
            busy: false,
            error: None,
            _subscriptions: subscription.into_iter().collect(),
        };
        // Start at the server's home directory (browse(None) resolves it
        // server-side), so the list is useful immediately without the user
        // having to navigate anywhere first.
        dialog.browse_folder(None, cx);
        dialog
    }

    fn set_mode(&mut self, mode: NewProjectMode, cx: &mut Context<Self>) {
        self.mode = mode;
        // Switching cards clears the previous attempt's error: leaving it up
        // would read as a complaint about the mode the user just left.
        self.error = None;
        cx.notify();
    }

    /// Close the dialog and open Settings on the Providers tab, where the GitHub
    /// token field lives.
    ///
    /// Closes this window first: leaving it open behind the settings window would
    /// put a second window with a live form in front of the user after they return.
    fn open_providers(&self, window: &mut Window, cx: &mut Context<Self>) {
        if let Some(app) = self.app.upgrade() {
            app.update(cx, |app_state, cx| {
                app_state.close_new_project_dialog(cx);
                app_state.open_settings_tab(
                    console_ui::settings::SettingsTab::Providers,
                    window,
                    cx,
                );
            });
        }
    }

    fn close(&self, cx: &mut Context<Self>) {
        if let Some(app) = self.app.upgrade() {
            app.update(cx, |app_state, cx| app_state.close_new_project_dialog(cx));
        }
    }

    /// List `path` (server home when `None`) into the folder browser.
    ///
    /// This hits `/api/fs/browse`, which runs on the *server* — so the paths
    /// shown are the ones that actually exist where the project will be
    /// created. That is the whole point of this widget: a native macOS panel
    /// would browse the Mac, which is the wrong filesystem whenever the
    /// backend is a remote host.
    fn browse_folder(&mut self, path: Option<String>, cx: &mut Context<Self>) {
        let Some(app) = self.app.upgrade() else {
            return;
        };
        let client = app.read(cx).client.clone();
        self.folder_loading = true;
        cx.notify();

        let entity = cx.entity().downgrade();
        cx.spawn(async move |_this, cx| {
            let result = client.fs.browse(path.as_deref()).await;
            let Some(dialog) = entity.upgrade() else {
                return;
            };
            cx.update(|cx| {
                dialog.update(cx, |this, cx| {
                    this.folder_loading = false;
                    match result {
                        Ok(listing) => {
                            this.folder_path = Some(listing.current_path);
                            this.folder_parent = listing.parent_path;
                            // Folders only: this widget exists to choose a
                            // directory, so files would just be noise.
                            this.folder_entries =
                                listing.entries.into_iter().filter(|e| e.is_dir).collect();
                        }
                        // A browse failure must not silently empty the list —
                        // that reads as "this folder has no subfolders".
                        Err(error) => {
                            this.error = Some((FailureKind::Message, error.to_string()));
                        }
                    }
                    cx.notify();
                });
            });
        })
        .detach();
    }

    /// Single click: highlight a row without navigating.
    fn select_folder(&mut self, path: String, cx: &mut Context<Self>) {
        self.folder_selected = Some(path);
        cx.notify();
    }

    /// Commit the highlighted folder (or the one being listed, when nothing is
    /// highlighted) as the project folder.
    fn use_folder(&mut self, cx: &mut Context<Self>) {
        let chosen = self
            .folder_selected
            .clone()
            .or_else(|| self.folder_path.clone());
        let Some(chosen) = chosen else {
            return;
        };
        self.parent_dir = chosen;
        // A stale error would still describe the previous attempt.
        self.error = None;
        cx.notify();
    }

    fn fail(&mut self, kind: FailureKind, message: String, cx: &mut Context<Self>) {
        self.busy = false;
        self.error = Some((kind, message));
        cx.notify();
    }

    /// Run the chosen bootstrap, then hand the project to the app.
    ///
    /// Client-side checks only cover empty fields, to avoid a pointless round
    /// trip. Everything that touches the filesystem is the server's call, and
    /// it re-validates regardless — it owns the directory being created.
    fn submit(&mut self, cx: &mut Context<Self>) {
        if self.busy {
            return;
        }
        let name = self.name_input.read(cx).content().trim().to_string();
        let parent = self.parent_dir.trim().to_string();
        let url = self.url_input.read(cx).content().trim().to_string();

        let missing = if parent.is_empty() {
            Some("Choose a folder for the project.")
        } else if self.mode == NewProjectMode::Blank && name.is_empty() {
            Some("Enter a project name.")
        } else if self.mode == NewProjectMode::Clone && url.is_empty() {
            Some("Enter a repository URL.")
        } else {
            None
        };
        if let Some(message) = missing {
            self.fail(FailureKind::Message, message.to_string(), cx);
            return;
        }

        let Some(app) = self.app.upgrade() else {
            return;
        };
        let client = app.read(cx).client.clone();
        self.busy = true;
        self.error = None;
        cx.notify();

        let mode = self.mode;
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_this, cx| {
            let result = match mode {
                NewProjectMode::Blank => client.projects.init_blank(&name, &parent).await,
                NewProjectMode::Clone => {
                    // An empty folder name is passed as None so the server
                    // derives one from the repo URL instead of failing on "".
                    let folder = if name.is_empty() {
                        None
                    } else {
                        Some(name.as_str())
                    };
                    client.projects.clone_project(&url, &parent, folder).await
                }
            };

            let Some(dialog) = entity.upgrade() else {
                return;
            };
            cx.update(|cx| match result {
                // Registers the project, selects it, and closes the dialog in
                // one step, so a created project can never leave its dialog
                // sitting open behind it.
                Ok(project) => dialog.update(cx, |_this, cx| {
                    app.update(cx, |app_state, cx| {
                        app_state.adopt_bootstrapped_project(project, cx)
                    });
                }),
                Err(error) => {
                    let kind = if is_github_not_connected(&error) {
                        FailureKind::NotConnected
                    } else {
                        FailureKind::Message
                    };
                    dialog.update(cx, |this, cx| this.fail(kind, error.to_string(), cx));
                }
            });
        })
        .detach();
    }
}

impl Focusable for NewProjectDialog {
    fn focus_handle(&self, _cx: &App) -> FocusHandle {
        self.focus_handle.clone()
    }
}

impl Render for NewProjectDialog {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let theme = Theme::current(cx);
        let busy = self.busy;
        let mode = self.mode;

        let blank_pick: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity();
            Rc::new(move |_w, cx| {
                entity.update(cx, |this, cx| this.set_mode(NewProjectMode::Blank, cx));
            })
        };
        let clone_pick: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity();
            Rc::new(move |_w, cx| {
                entity.update(cx, |this, cx| this.set_mode(NewProjectMode::Clone, cx));
            })
        };
        let cancel: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity();
            Rc::new(move |_w, cx| {
                entity.update(cx, |this, cx| this.close(cx));
            })
        };
        let submit: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity();
            Rc::new(move |_w, cx| {
                entity.update(cx, |this, cx| this.submit(cx));
            })
        };

        div()
            .size_full()
            .bg(theme.canvas)
            .track_focus(&self.focus_handle)
            // Enter submits from anywhere in the dialog; it never reaches the
            // inputs first because they live in their own focus scopes and
            // this handler only fires when the dialog itself holds focus.
            .on_key_down(cx.listener(|this, event: &KeyDownEvent, _window, cx| {
                match event.keystroke.key.as_str() {
                    "escape" => this.close(cx),
                    "enter" => this.submit(cx),
                    _ => {}
                }
            }))
            .child(
                div()
                    .mx(px(20.0))
                    .mt(px(52.0))
                    .flex()
                    .flex_col()
                    .gap(px(14.0))
                    .child(
                        div()
                            .text_size(px(17.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child("New Project"),
                    )
                    .child(
                        div()
                            .flex()
                            .gap(px(10.0))
                            .child(mode_card(
                                "Blank project",
                                "Create a folder and start a git repository.",
                                mode == NewProjectMode::Blank,
                                busy,
                                blank_pick,
                                &theme,
                            ))
                            .child(mode_card(
                                "Clone from GitHub",
                                "Copy a repository from GitHub into a new folder.",
                                mode == NewProjectMode::Clone,
                                busy,
                                clone_pick,
                                &theme,
                            )),
                    )
                    .child(folder_browser(
                        self.folder_path.clone(),
                        self.folder_parent.clone(),
                        self.folder_entries.clone(),
                        self.folder_selected.clone(),
                        self.parent_dir.clone(),
                        self.folder_loading,
                        cx.entity().downgrade(),
                        &theme,
                    ))
                    // The clone card asks for a URL; the blank card has no
                    // such field. Only one of these blocks is ever mounted, so
                    // the blank card cannot be submitted with a stale URL left
                    // in a hidden input.
                    .child(if mode == NewProjectMode::Blank {
                        field("Name", self.name_input.clone(), &theme)
                    } else {
                        div()
                            .flex()
                            .flex_col()
                            .gap(px(12.0))
                            .child(field("Repository URL", self.url_input.clone(), &theme))
                            .child(field(
                                "Folder name (optional)",
                                self.name_input.clone(),
                                &theme,
                            ))
                    })
                    .child(match self.error.clone() {
                        Some((kind, message)) => error_row(kind, &message, &theme, cx),
                        None => div(),
                    })
                    .child(
                        div()
                            .flex()
                            .gap(px(8.0))
                            .justify_end()
                            .child(button("Cancel", false, busy, cancel, &theme))
                            .child(button(
                                if busy {
                                    "Working…"
                                } else if mode == NewProjectMode::Blank {
                                    "Create project"
                                } else {
                                    "Clone repository"
                                },
                                true,
                                busy,
                                submit,
                                &theme,
                            )),
                    ),
            )
    }
}

/// One of the two option cards. Selecting is by click rather than radio so the
/// whole card is the target, which is easier to hit than a small control.
fn mode_card(
    title: &'static str,
    blurb: &'static str,
    selected: bool,
    busy: bool,
    on_pick: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    theme: &Theme,
) -> impl IntoElement {
    let card = div()
        .flex_1()
        .id(gpui::ElementId::from(format!("new-project-card-{title}")))
        .p(px(12.0))
        .rounded(px(8.0))
        .border_1()
        .border_color(if selected { theme.accent } else { theme.border })
        .bg(if selected {
            theme.surface
        } else {
            theme.canvas
        })
        .flex()
        .flex_col()
        .gap(px(4.0))
        .opacity(if busy { 0.6 } else { 1.0 })
        .on_mouse_down(MouseButton::Left, move |_e, window, cx| {
            if !busy {
                cx.stop_propagation();
                on_pick(window, cx);
            }
        })
        .child(
            div()
                .text_size(px(13.0))
                .font_weight(gpui::FontWeight::SEMIBOLD)
                .text_color(theme.text)
                .child(title),
        )
        .child(
            div()
                .text_size(px(11.5))
                .text_color(theme.text_secondary)
                .child(blurb),
        );
    // While a request is in flight the cards are inert, so they neither show
    // a pointer nor respond to hover — a dimmed-but-clickable card would
    // imply the mode is switchable mid-request.
    if busy {
        card.into_any_element()
    } else {
        card.cursor_pointer()
            .hover(|s| s.opacity(0.9))
            .into_any_element()
    }
}

/// A labelled field with a visible border around the input.
///
/// The border lives here rather than on the input itself: `ComposerInput`
/// draws its own background, and restyling that shared widget would change
/// every composer in the app to fix one dialog.
fn field(label: &'static str, input: Entity<ComposerInput>, theme: &Theme) -> gpui::Div {
    div()
        .flex()
        .flex_col()
        .gap(px(6.0))
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child(label),
        )
        .child(
            div()
                .border_1()
                .border_color(theme.border)
                .bg(theme.surface)
                .rounded(px(6.0))
                .px(px(8.0))
                .py(px(6.0))
                .child(input),
        )
}

/// Inline folder browser for the SERVER's filesystem.
///
/// Replaces a text field plus a native panel on purpose: the backend may be a
/// remote host, where a macOS folder picker would offer paths that do not
/// exist where the project is actually created.
///
/// Interaction:
/// - **single click** a row  → highlight it (this is what "Use this folder" takes)
/// - **double click** a row  → list that folder
/// - **..**                  → go to the parent directory
/// - **Use this folder**     → commit the highlighted row, or the folder being
///                             listed when nothing is highlighted
#[allow(clippy::too_many_arguments)]
fn folder_browser(
    current: Option<String>,
    parent: Option<String>,
    entries: Vec<FsTreeEntry>,
    selected: Option<String>,
    chosen: String,
    loading: bool,
    entity: gpui::WeakEntity<NewProjectDialog>,
    theme: &Theme,
) -> gpui::Div {
    let mut body = div().flex().flex_col().gap(px(6.0)).child(
        // Header: what is listed, and what has been chosen. Showing the
        // committed folder separately matters — it is the value the server
        // will use, and it is often not the row currently highlighted.
        div()
            .flex()
            .items_center()
            .justify_between()
            .gap(px(8.0))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(2.0))
                    .child(
                        div()
                            .text_size(px(12.0))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.text_secondary)
                            .child("Folder on the server"),
                    )
                    .child(
                        div()
                            .text_size(px(11.0))
                            .text_color(theme.text_ghost)
                            .child(if chosen.is_empty() {
                                "No folder chosen yet".to_string()
                            } else {
                                chosen.clone()
                            }),
                    ),
            )
            .child({
                let use_entity = entity.clone();
                let use_button = div()
                    .id("btn-use-this-folder")
                    .px(px(10.0))
                    .py(px(4.0))
                    .rounded(px(5.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.raised)
                    .child(
                        div()
                            .text_size(px(11.5))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.text)
                            .child("Use this folder"),
                    )
                    .on_mouse_down(MouseButton::Left, move |_e, _w, cx| {
                        cx.stop_propagation();
                        if let Some(dialog) = use_entity.upgrade() {
                            dialog.update(cx, |this, cx| this.use_folder(cx));
                        }
                    });
                // Inert only before the first listing lands; after that there
                // is always something to use (the folder being listed).
                if loading && current.is_none() {
                    use_button.opacity(0.5).into_any_element()
                } else {
                    use_button.cursor_pointer().into_any_element()
                }
            }),
    );

    // The listing, scrollable so a home directory with hundreds of entries does
    // not push the URL and action rows off the dialog.
    let mut list = div()
        // `.id` is what makes the element stateful, and a stateful element is
        // what provides `overflow_y_scroll` — without it the method does not
        // exist on Div.
        .id("new-project-folder-list")
        .flex()
        .flex_col()
        .border_1()
        .border_color(theme.border)
        .bg(theme.surface)
        .rounded(px(6.0))
        .overflow_y_scroll()
        .max_h(px(150.0));

    if let Some(parent) = parent.clone() {
        list = list.child(folder_row(
            "..",
            "Parent folder",
            false,
            false,
            Some(parent),
            entity.clone(),
            theme,
        ));
    }

    // Borrowed rather than consumed: `folder_row` needs the name and path as
    // separate `&str`s, and `entries.is_empty()` below must still be callable.
    for entry in &entries {
        let is_selected = selected.as_deref() == Some(entry.path.as_str());
        list = list.child(folder_row(
            &entry.name,
            &entry.path,
            is_selected,
            true,
            Some(entry.path.clone()),
            entity.clone(),
            theme,
        ));
    }

    // Empty/loading states. Showing "No folders" while a request is in flight
    // would flash before every navigation, so they are kept distinct.
    if entries.is_empty() && parent.is_none() {
        list = list.child(
            div()
                .px(px(8.0))
                .py(px(10.0))
                .text_size(px(11.5))
                .text_color(theme.text_ghost)
                .child(if loading {
                    "Loading…"
                } else {
                    "No folders to show."
                }),
        );
    }

    body = body.child(list);

    if let Some(current) = current {
        body = body.child(
            div()
                .text_size(px(10.5))
                .text_color(theme.text_ghost)
                .child(format!(
                    "Listing {current} — double-click a folder to open it"
                )),
        );
    }

    body
}

/// One row. `is_folder` is false only for the ".." row, which navigates on a
/// single click instead of requiring a double: going up is a one-off action,
/// and making it double-click-only would be needlessly fiddly.
#[allow(clippy::too_many_arguments)]
fn folder_row(
    label: &str,
    detail: &str,
    selected: bool,
    is_folder: bool,
    path: Option<String>,
    entity: gpui::WeakEntity<NewProjectDialog>,
    theme: &Theme,
) -> impl IntoElement {
    let row = div()
        .id(gpui::ElementId::from(format!("folder-row-{}", label)))
        .px(px(8.0))
        .py(px(5.0))
        .flex()
        .flex_col()
        .gap(px(1.0))
        .cursor_pointer()
        .on_mouse_down(MouseButton::Left, {
            let path = path.clone();
            let entity = entity.clone();
            move |event, _window, cx| {
                cx.stop_propagation();
                let Some(dialog) = entity.upgrade() else {
                    return;
                };
                // A double click enters the folder; a single click only
                // highlights it. The ".." row ignores the distinction.
                if is_folder && event.click_count >= 2 {
                    dialog.update(cx, |this, cx| this.browse_folder(path.clone(), cx));
                } else if is_folder {
                    dialog.update(cx, |this, cx| {
                        this.select_folder(path.clone().unwrap_or_default(), cx)
                    });
                } else if let Some(parent) = path.clone() {
                    dialog.update(cx, |this, cx| this.browse_folder(Some(parent), cx));
                }
            }
        })
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(if selected { theme.accent } else { theme.text })
                .child(label.to_string()),
        )
        .child(
            div()
                .text_size(px(10.5))
                .text_color(theme.text_ghost)
                .child(detail.to_string()),
        );

    if selected {
        row.bg(theme.overlay).into_any_element()
    } else {
        row.hover(|s| s.bg(theme.overlay)).into_any_element()
    }
}

/// Error text. The not-connected case gets a second line pointing at the
/// screen that can actually fix it, because the server's message names
/// Accounts but the dialog cannot navigate there itself.
fn error_row(
    kind: FailureKind,
    message: &str,
    theme: &Theme,
    cx: &mut Context<NewProjectDialog>,
) -> gpui::Div {
    let mut row = div()
        .p(px(10.0))
        .rounded(px(6.0))
        .border_1()
        .border_color(theme.border)
        .bg(theme.surface)
        .flex()
        .flex_col()
        .gap(px(4.0))
        .child(
            div()
                .text_size(px(12.0))
                .text_color(theme.text_secondary)
                .child(message.to_string()),
        );
    if kind == FailureKind::NotConnected {
        row = row.child(
            div()
                .flex()
                .items_center()
                .gap(px(8.0))
                .child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.text_ghost)
                        .child("Public repositories can be cloned without a token."),
                )
                .child({
                    // A real button, not just prose: the fix lives in another
                    // window, so telling the user where to go without offering
                    // to take them there is a dead end.
                    let open_providers: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
                        let entity = cx.entity();
                        Rc::new(move |window, cx| {
                            entity.update(cx, |this, cx| this.open_providers(window, cx));
                        })
                    };
                    div()
                        .id("btn-new-project-open-providers")
                        .px(px(8.0))
                        .py(px(3.0))
                        .rounded(px(4.0))
                        .border_1()
                        .border_color(theme.border)
                        .cursor_pointer()
                        .hover(|s| s.opacity(0.9))
                        .child(
                            div()
                                .text_size(px(11.5))
                                .font_weight(gpui::FontWeight::MEDIUM)
                                .text_color(theme.text)
                                .child("Open GitHub settings"),
                        )
                        .on_mouse_down(MouseButton::Left, move |_e, window, cx| {
                            cx.stop_propagation();
                            open_providers(window, cx);
                        })
                        .into_any_element()
                }),
        );
    }
    row
}

fn button(
    label: &str,
    primary: bool,
    disabled: bool,
    on_click: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    theme: &Theme,
) -> impl IntoElement {
    let button = div()
        .id(gpui::ElementId::from(format!("new-project-btn-{label}")))
        .px(px(12.0))
        .py(px(6.0))
        .rounded(px(6.0))
        .bg(if primary { theme.accent } else { theme.surface })
        .border_1()
        .border_color(if primary { theme.accent } else { theme.border })
        .opacity(if disabled { 0.5 } else { 1.0 })
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(if primary {
                    theme.on_inverse
                } else {
                    theme.text
                })
                .child(label.to_string()),
        )
        .on_mouse_down(MouseButton::Left, move |_e, window, cx| {
            if !disabled {
                cx.stop_propagation();
                on_click(window, cx);
            }
        });
    if disabled {
        button.into_any_element()
    } else {
        button.cursor_pointer().into_any_element()
    }
}
