//! The GitHub section shared by the two project menus — the docked footer's
//! project dropdown and the floating composer card's. One builder, so the
//! rows ("Loading repositories…", the connect handoff, Retry) behave
//! identically everywhere a project is picked.
//!
//! The parent owns all backend calls; this only paints state and forwards
//! callbacks, like the menus that embed it.

use std::rc::Rc;

use console_core::GithubRepo;
use gpui::{App, Window};

use crate::primitives::{IconName, MenuItem};

#[allow(clippy::type_complexity)]
pub fn github_menu_section(
    repos: Option<Rc<Vec<GithubRepo>>>,
    loading: bool,
    has_error: bool,
    on_open: &Rc<dyn Fn(GithubRepo, &mut Window, &mut App) + 'static>,
    on_refresh: &Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    on_connect: &Rc<dyn Fn(&mut Window, &mut App) + 'static>,
) -> Vec<MenuItem> {
    let mut items = vec![MenuItem::Header("GitHub".into())];
    if loading {
        items.push(MenuItem::new("Loading repositories…", |_, _| {}).disabled(true));
        return items;
    }
    if let Some(repos) = repos {
        if repos.is_empty() {
            items.push(MenuItem::new("No repositories found", |_, _| {}).disabled(true));
        }
        for repo in repos.iter().take(20) {
            let repo = repo.clone();
            items.push(
                MenuItem::new(repo.name.clone(), {
                    let on_open = on_open.clone();
                    move |window, cx| {
                        (on_open)(repo.clone(), window, cx);
                    }
                })
                .icon(IconName::Github.path()),
            );
        }
        items.push(
            MenuItem::new("Refresh", {
                let on_refresh = on_refresh.clone();
                move |window, cx| {
                    (on_refresh)(window, cx);
                }
            })
            .icon(IconName::Refresh.path()),
        );
        return items;
    }
    if has_error {
        // A real failure (network, server). The text is too long for a menu
        // row — it is logged by the caller — so the row stays inert and the
        // retry sits under it.
        items.push(MenuItem::new("Couldn't load repositories", |_, _| {}).disabled(true));
        items.push(
            MenuItem::new("Retry", {
                let on_refresh = on_refresh.clone();
                move |window, cx| {
                    (on_refresh)(window, cx);
                }
            })
            .icon(IconName::Refresh.path()),
        );
        return items;
    }
    // Not connected: the fetch reports this as a state, not an error. Offer
    // the handoff to Settings > Providers.
    items.push(
        MenuItem::new("Connect GitHub…", {
            let on_connect = on_connect.clone();
            move |window, cx| {
                (on_connect)(window, cx);
            }
        })
        .icon(IconName::Github.path()),
    );
    items
}
