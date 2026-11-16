//! The workspace tab bar's `+` dropdown: one handle per pane, and the rows
//! that open each kind of tab in the pane whose button was clicked.

use std::rc::Rc;

use console_ui::{ContextMenuHandle, IconName, MenuItem};
use gpui::{App, Context, WeakEntity, Window};

use crate::state::app::ConsoleDesktopApp;

fn kbd(mac: &'static str) -> Option<&'static str> {
    cfg!(target_os = "macos").then_some(mac)
}

impl ConsoleDesktopApp {
    /// Make sure every pane has a menu handle; the tab bar only has `&App`.
    pub(crate) fn ensure_new_tab_menus(&mut self, cx: &mut Context<Self>) {
        let pane_ids: Vec<String> = self
            .workspace_root
            .leaves()
            .iter()
            .map(|leaf| leaf.id.clone())
            .collect();
        for id in &pane_ids {
            if !self.new_tab_menus.contains_key(id) {
                self.new_tab_menus
                    .insert(id.clone(), ContextMenuHandle::new(cx));
            }
        }
        self.new_tab_menus.retain(|id, _| pane_ids.contains(id));
    }
}

pub(crate) fn new_tab_menu_items(
    entity: WeakEntity<ConsoleDesktopApp>,
    pane_id: String,
) -> Rc<dyn Fn(&mut App) -> Vec<MenuItem>> {
    Rc::new(move |_cx| {
        let row = |label: &'static str,
                   icon: IconName,
                   shortcut: Option<&'static str>,
                   run: Rc<dyn Fn(&mut ConsoleDesktopApp, &mut Window, &mut Context<ConsoleDesktopApp>)>| {
            let entity = entity.clone();
            let pane_id = pane_id.clone();
            let item = MenuItem::new(label, move |window, cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.active_pane_id = Some(pane_id.clone());
                        run(this, window, cx);
                    });
                }
            })
            .icon(icon.path());
            match shortcut {
                Some(s) => item.shortcut(s),
                None => item,
            }
        };

        let mut items = vec![
            row(
                "New Chat",
                IconName::ChatRoundLine,
                kbd("⌃N"),
                Rc::new(|this, window, cx| this.open_floating_composer(window, cx)),
            ),
            row(
                "Files",
                IconName::FolderOpen,
                kbd("⌃F"),
                Rc::new(|this, _, cx| this.open_file_tree_tab(cx)),
            ),
            row(
                "Changes",
                IconName::FileDiff,
                kbd("⌃G"),
                Rc::new(|this, _, cx| this.open_changes_for_active_pane(cx)),
            ),
            row("Terminal", IconName::Terminal, kbd("⌃T"), {
                let pane_id = pane_id.clone();
                Rc::new(move |this, window, cx| {
                    this.open_terminal_tab_for_pane_project(&pane_id, window, cx)
                })
            }),
        ];
        if !cfg!(target_os = "windows") {
            let pane_id = pane_id.clone();
            items.push(row(
                "Browser",
                IconName::Global,
                kbd("⌃B"),
                Rc::new(move |this, window, cx| {
                    this.open_browser_tab_in_pane(&pane_id, None, true, window, cx)
                }),
            ));
        }
        if cfg!(target_os = "macos") {
            items.push(row(
                "Devices",
                IconName::Smartphone,
                kbd("⌃D"),
                Rc::new(|this, _, cx| this.open_device_tab(cx)),
            ));
        }
        items
    })
}

impl ConsoleDesktopApp {
    /// Open the changes review for the chat in the active pane, if any.
    pub fn open_changes_for_active_pane(&mut self, cx: &mut Context<Self>) {
        let pane_id = self
            .active_pane_id
            .clone()
            .unwrap_or_else(|| "pane-main".into());
        if let Some(session_id) = self.active_session_for_pane(&pane_id) {
            self.open_changes_review_tab(
                session_id,
                console_core::types::ChangesScope::AllTurns,
                cx,
            );
        }
    }
}
