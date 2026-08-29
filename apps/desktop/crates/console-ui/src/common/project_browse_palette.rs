//! ⌘O project browser palette, built on the central [`crate::CommandPaletteModal`].
//!
//! Lists the remote backend's filesystem (starting at the user's home),
//! lets the user drill into folders, and confirms a folder as a tracked
//! project — the same flow a native folder picker would provide, but against
//! the host the server runs on.
//!
//! Interaction:
//! - **Enter / click on a folder** → use that folder as the project
//! - **▶ chevron on a folder** → drill into that folder
//! - **Enter / click on ".."** → go to the parent directory
//! - **Enter / click on "Use this folder"** → register `current_path` as a project
//! - Type to filter the listing locally

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
    /// Parent of `current_path` (from the last browse response), if any.
    parent_path: Option<String>,
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
            parent_path: None,
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
            modal.set_placeholder("Filter folders…", cx);
            // Local filter against the listing; query does not re-fetch.
            modal.set_filterable(true, cx);
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

        // Clear the list while loading so the previous folder doesn't linger.
        modal.update(cx, |m, cx| m.set_entries(Vec::new(), cx));

        cx.spawn(async move |this, cx| {
            let resp = match path.as_deref() {
                Some(path) => client.fs.browse(Some(path)).await,
                None => client.fs.browse(None).await,
            };
            match resp {
                Ok(resp) => {
                    let _ = this.update(cx, |this, cx| {
                        this.current_path = Some(resp.current_path.clone());
                        this.parent_path = resp.parent_path.clone();
                        this.modal.update(cx, |m, cx| {
                            // Show the current path in the placeholder so the
                            // user always knows where they are.
                            m.set_placeholder(
                                format!("Filter in {}…", short_path(&resp.current_path)),
                                cx,
                            );
                            m.set_entries(
                                entries_from_browse(
                                    resp.current_path,
                                    resp.parent_path,
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
                    let _ = this.update(cx, |this, cx| {
                        this.modal.update(cx, |m, cx| m.set_entries(Vec::new(), cx));
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
        self.modal.clone()
    }
}

/// Shorten a path for the placeholder (home → ~, otherwise basename tail).
fn short_path(path: &str) -> String {
    if let Some(home) = std::env::var_os("HOME").and_then(|h| h.into_string().ok()) {
        if let Some(rest) = path.strip_prefix(&home) {
            if rest.is_empty() {
                return "~".into();
            }
            return format!("~{rest}");
        }
    }
    path.to_string()
}

/// Build a row that confirms `path` as the project and closes the palette.
fn select_folder_entry(
    path: String,
    label: String,
    icon: IconName,
    modal: &Entity<CommandPaletteModal>,
    on_select_project: &Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
) -> PaletteEntry {
    let modal = modal.clone();
    let on_select_project = on_select_project.clone();
    PaletteEntry::new(format!("select:{path}"), label, move |window, cx| {
        if let Some(on_select_project) = &on_select_project {
            on_select_project(path.clone(), window, cx);
        }
        modal.update(cx, |modal, cx| modal.hide(cx));
    })
    .icon(icon)
}

/// Rows for a directory listing:
/// 1. "Use this folder" — Enter/click registers `current_path` as a project
/// 2. ".." parent (when available) — Enter/click drills up
/// 3. Subfolders — Enter/click **selects** that folder as the project;
///    trailing ▶ chevron drills into it
///
/// Files are omitted: this picker is for choosing a project root, not opening files.
fn entries_from_browse(
    current_path: String,
    parent_path: Option<String>,
    entries: Vec<FsEntry>,
    modal: &Entity<CommandPaletteModal>,
    on_select_project: &Option<Rc<dyn Fn(String, &mut Window, &mut App)>>,
) -> Vec<PaletteEntry> {
    let mut out = Vec::with_capacity(entries.len() + 2);

    // Confirm current folder (Enter on this row adds the project).
    let folder_label = format!("Use this folder · {}", short_path(&current_path));
    out.push(select_folder_entry(
        current_path,
        folder_label,
        IconName::FolderOpen,
        modal,
        on_select_project,
    ));

    // Parent directory — pure navigation.
    if let Some(parent) = parent_path {
        out.push(
            PaletteEntry::new(parent, "..", |_window, _cx| {})
                .icon(IconName::Folder)
                .keep_open(true),
        );
    }

    // Subfolders: Enter/click selects; ▶ drills in.
    // Cap the listing so huge home directories stay snappy in the palette.
    const MAX_DIRS: usize = 200;
    let mut dirs: Vec<FsEntry> = entries.into_iter().filter(|e| e.is_dir).collect();
    dirs.sort_by(|a, b| a.name.to_ascii_lowercase().cmp(&b.name.to_ascii_lowercase()));
    dirs.truncate(MAX_DIRS);
    for entry in dirs {
        let path = entry.path.clone();
        out.push(
            select_folder_entry(
                entry.path,
                entry.name,
                IconName::Folder,
                modal,
                on_select_project,
            )
            .drill_into(path),
        );
    }

    out
}
