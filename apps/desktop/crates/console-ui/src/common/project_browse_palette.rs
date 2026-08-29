//! ⌘O project browser palette, built on the central [`crate::CommandPaletteModal`].
//!
//! Lists the remote backend's filesystem (starting at the user's home),
//! lets the user drill into folders, and confirms the current folder as a
//! tracked project — the same flow a native folder picker would provide, but
//! against the host the server runs on.

use std::rc::Rc;

use console_core::{ConsoleClient, FsEntry};
use gpui::{App, AppContext, Context, Entity, IntoElement, Render, Window};

use crate::CommandPaletteModal;
use crate::IconName;
use crate::PaletteEntry;

pub struct ProjectBrowsePalette {
    modal: Entity<CommandPaletteModal>,
    client: ConsoleClient,
    /// The directory whose contents are currently listed.
    current_path: Option<String>,
    /// Folder-confirm callback: receives the folder to register as a project.
    on_select_project: Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
}

impl ProjectBrowsePalette {
    pub fn new(
        client: ConsoleClient,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        let modal = cx.new(|cx| CommandPaletteModal::new(window, cx));
        Self {
            modal,
            client,
            current_path: None,
            on_select_project: None,
        }
    }

    /// Set the callback invoked with the confirmed folder path.
    pub fn set_on_select_project(
        &mut self,
        callback: impl Fn(String, &mut Window, &mut App) + 'static,
        _cx: &mut Context<Self>,
    ) {
        self.on_select_project = Some(Rc::new(callback));
    }

    /// Open the browser at the user's home directory.
    pub fn open(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let this = cx.entity().downgrade();
        self.modal.update(cx, |modal, cx| {
            modal.set_placeholder("Type to filter…", cx);
            modal.set_query_handler(|_query, _window, _cx| {}, cx);
            modal.set_browse_callback(
                {
                    let this = this.clone();
                    move |path, _window, cx| {
                        if let Some(this) = this.upgrade() {
                            this.update(cx, |this, cx| this.browse(Some(path), cx));
                        }
                    }
                },
                cx,
            );
            modal.set_entries(Vec::new(), cx);
            modal.show(window, cx);
        });
        self.browse(None, cx);
    }

    pub fn hide(&mut self, cx: &mut Context<Self>) {
        self.modal.update(cx, |modal, cx| modal.hide(cx));
    }

    pub fn is_open(&self, cx: &App) -> bool {
        self.modal.read(cx).is_open()
    }

    /// List `path` (home when `None`) and replace the palette's rows.
    fn browse(&mut self, path: Option<&str>, cx: &mut Context<Self>) {
        self.current_path = path.map(str::to_string);
        let modal = self.modal.clone();
        let client = self.client.clone();
        let path = path.map(str::to_string);
        let on_select_project = self.on_select_project.clone();

        cx.spawn(async move |_, cx| {
            let resp = match path.as_deref() {
                Some(path) => client.fs.browse(Some(path)).await,
                None => client.fs.browse(None).await,
            };
            match resp {
                Ok(resp) => {
                    cx.update(|cx| {
                        let modal = modal.clone();
                        modal.update(cx, |m, cx| {
                            m.set_entries(
                                entries_from_browse(
                                    resp.current_path,
                                    resp.entries,
                                    &modal,
                                    &on_select_project,
                                ),
                                cx,
                            );
                        });
                    });
                }
                Err(_) => {
                    cx.update(|cx| {
                        modal.update(cx, |m, cx| m.set_entries(Vec::new(), cx));
                    });
                }
            }
        })
        .detach();
    }
}

impl Render for ProjectBrowsePalette {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        // Return the modal entity directly — same as CommandPalette.
        // Wrapping in an empty `div()` makes the absolute overlay size against
        // a 0×0 parent (GPUI/Taffy absolute is relative to the parent node),
        // so the palette never appears.
        self.modal.clone()
    }
}

/// Rows for a directory listing: a "select this folder" action up top, then
/// folders (navigate in place) and files.
fn entries_from_browse(
    current_path: String,
    entries: Vec<FsEntry>,
    modal: &Entity<CommandPaletteModal>,
    on_select_project: &Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
) -> Vec<PaletteEntry> {
    let modal = modal.clone();
    let on_select_project = on_select_project.clone();
    let mut out = Vec::with_capacity(entries.len() + 1);

    // "Select Current Folder as Project" — always the first row.
    out.push(
        PaletteEntry::new(
            format!("select:{}", current_path),
            "Select this folder as project",
            {
                let modal = modal.clone();
                let on_select_project = on_select_project.clone();
                let path = current_path.clone();
                move |window, cx| {
                    if let Some(on_select_project) = &on_select_project {
                        on_select_project(path.clone(), window, cx);
                    }
                    modal.update(cx, |modal, cx| modal.hide(cx));
                }
            },
        )
        .icon(IconName::FolderOpen),
    );

    for entry in entries {
        let path = entry.path;
        let name = entry.name;
        if entry.is_directory {
            out.push(
                PaletteEntry::new(path, name, |_window, _cx| {})
                    .icon(IconName::Folder)
                    .keep_open(true),
            );
        } else {
            out.push(PaletteEntry::new(path, name, |_window, _cx| {}).icon(IconName::File));
        }
    }

    out
}
