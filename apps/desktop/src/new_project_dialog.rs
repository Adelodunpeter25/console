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
use console_ui::input::ComposerInput;
use console_ui::theme::Theme;
use gpui::{
    App, AppContext, Context, Entity, FocusHandle, Focusable, InteractiveElement, IntoElement,
    KeyDownEvent, MouseButton, ParentElement, Render, Styled, Window, div, px,
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
    parent_input: Entity<ComposerInput>,
    url_input: Entity<ComposerInput>,
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
        // Read-only: the folder comes from Finder, never from typing. A
        // mistyped path here would otherwise surface much later as an opaque
        // server error instead of at the point of the mistake.
        let parent_input = cx.new(|cx| {
            ComposerInput::new(window, cx)
                .placeholder("Choose a folder…")
                .read_only(true)
        });
        let url_input =
            cx.new(|cx| ComposerInput::new(window, cx).placeholder("https://github.com/org/repo"));

        let focus_handle = cx.focus_handle();
        window.focus(&focus_handle, cx);

        Self {
            app,
            focus_handle,
            mode: NewProjectMode::Blank,
            name_input,
            parent_input,
            url_input,
            busy: false,
            error: None,
            _subscriptions: subscription.into_iter().collect(),
        }
    }

    fn set_mode(&mut self, mode: NewProjectMode, cx: &mut Context<Self>) {
        self.mode = mode;
        // Switching cards clears the previous attempt's error: leaving it up
        // would read as a complaint about the mode the user just left.
        self.error = None;
        cx.notify();
    }

    fn close(&self, cx: &mut Context<Self>) {
        if let Some(app) = self.app.upgrade() {
            app.update(cx, |app_state, cx| app_state.close_new_project_dialog(cx));
        }
    }

    /// Open the native folder picker and print the chosen path into the folder
    /// field.
    ///
    /// Async on purpose. A blocking `runModal`-based picker would spin a
    /// nested AppKit loop that re-enters GPUI's dispatcher while this
    /// `entity.update` still holds the `App` RefCell borrow, panicking on a
    /// double borrow — see the module comment in picker.rs. Awaiting the
    /// panel instead means no borrow is held while it is open.
    fn choose_folder(&mut self, cx: &mut Context<Self>) {
        if self.busy {
            return;
        }
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_this, cx| {
            // Opening the panel is synchronous and non-blocking (it just hands
            // AppKit a completion handler); the await below is what yields, and
            // it is polled by the dispatcher with no App borrow held. The
            // `update` only exists to reach the `&mut App` needed to show it.
            let picked = cx.update(|cx| crate::picker::pick_folder(cx));
            let Some(path) = crate::picker::resolve_single_path(picked).await else {
                // Cancelled: leave the previous folder and any error alone.
                return;
            };
            let Some(dialog) = entity.upgrade() else {
                return;
            };
            cx.update(|cx| {
                dialog.update(cx, |this, cx| {
                    this.parent_input
                        .update(cx, |input, cx| input.set_content(path, cx));
                    // A stale error would still describe the previous attempt.
                    this.error = None;
                    cx.notify();
                });
            });
        })
        .detach();
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
        let parent = self.parent_input.read(cx).content().trim().to_string();
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
        let choose_folder: Rc<dyn Fn(&mut Window, &mut App) + 'static> = {
            let entity = cx.entity();
            Rc::new(move |_w, cx| {
                entity.update(cx, |this, cx| this.choose_folder(cx));
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
                    .child(folder_field(
                        self.parent_input.clone(),
                        busy,
                        choose_folder,
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
                        Some((kind, message)) => error_row(kind, &message, &theme),
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

/// The folder field: read-only path plus a button that opens Finder.
///
/// Split from `field` because it is the one field whose value is chosen rather
/// than typed, and because the button needs to sit inside the border to read
/// as part of the control.
fn folder_field(
    input: Entity<ComposerInput>,
    disabled: bool,
    on_choose: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    theme: &Theme,
) -> gpui::Div {
    div()
        .flex()
        .flex_col()
        .gap(px(6.0))
        .child(
            div()
                .text_size(px(12.0))
                .font_weight(gpui::FontWeight::MEDIUM)
                .text_color(theme.text_secondary)
                .child("Folder"),
        )
        .child(
            div()
                .flex()
                .items_center()
                .gap(px(8.0))
                .border_1()
                .border_color(theme.border)
                .bg(theme.surface)
                .rounded(px(6.0))
                .px(px(8.0))
                .py(px(6.0))
                .child(div().flex_1().child(input))
                .child({
                    let choose = div()
                        .id("new-project-choose-folder")
                        .px(px(8.0))
                        .py(px(3.0))
                        .rounded(px(4.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.canvas)
                        .text_size(px(11.5))
                        .text_color(theme.text)
                        .opacity(if disabled { 0.5 } else { 1.0 })
                        .child("Choose…")
                        .on_mouse_down(MouseButton::Left, move |_e, window, cx| {
                            if !disabled {
                                cx.stop_propagation();
                                on_choose(window, cx);
                            }
                        });
                    if disabled {
                        choose.into_any_element()
                    } else {
                        choose.cursor_pointer().into_any_element()
                    }
                }),
        )
}

/// Error text. The not-connected case gets a second line pointing at the
/// screen that can actually fix it, because the server's message names
/// Accounts but the dialog cannot navigate there itself.
fn error_row(kind: FailureKind, message: &str, theme: &Theme) -> gpui::Div {
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
                .text_size(px(11.5))
                .text_color(theme.text_ghost)
                .child("Open Settings ▸ Accounts to add a token, then try again. Public repositories can be cloned without one."),
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
