use std::cell::RefCell;
use gpui::{
    AnyWindowHandle, App, AppContext, Bounds, TitlebarOptions, WeakEntity, WindowBounds,
    WindowOptions, point, px,
};

use crate::persistence;
use crate::state::ConsoleDesktopApp;

#[derive(Clone, Debug)]
pub enum WindowLaunchTarget {
    RestorePersisted,
    Fresh,
    Session(String),
}

thread_local! {
    static WORKSPACE_WINDOWS: RefCell<Vec<(AnyWindowHandle, WeakEntity<ConsoleDesktopApp>)>> =
        RefCell::new(Vec::new());
}

pub fn register_window(handle: AnyWindowHandle, app: WeakEntity<ConsoleDesktopApp>) {
    WORKSPACE_WINDOWS.with(|windows| {
        windows.borrow_mut().push((handle, app));
    });
}

pub fn get_active_window(
    cx: &mut App,
) -> Option<(AnyWindowHandle, gpui::Entity<ConsoleDesktopApp>)> {
    WORKSPACE_WINDOWS.with(|windows| {
        let mut list = windows.borrow_mut();
        list.retain(|(handle, app)| {
            app.upgrade().is_some() && handle.update(cx, |_, _, _| ()).is_ok()
        });

        for (handle, app) in list.iter().rev() {
            let is_active = handle
                .update(cx, |_, window, _| window.is_window_active())
                .unwrap_or(false);
            if is_active {
                if let Some(upgraded) = app.upgrade() {
                    return Some((*handle, upgraded));
                }
            }
        }

        if let Some((handle, app)) = list.last() {
            if let Some(upgraded) = app.upgrade() {
                return Some((*handle, upgraded));
            }
        }

        None
    })
}

pub fn compute_new_window_bounds(cx: &mut App) -> WindowBounds {
    let active_bounds = WORKSPACE_WINDOWS.with(|windows| {
        let list = windows.borrow();
        for (handle, _) in list.iter().rev() {
            let bounds = handle
                .update(cx, |_, window, _| match window.window_bounds() {
                    WindowBounds::Windowed(b) => Some(b),
                    WindowBounds::Maximized(b) => Some(b),
                    WindowBounds::Fullscreen(_) => None,
                })
                .ok()
                .flatten();
            if let Some(b) = bounds {
                return Some(b);
            }
        }
        None
    });

    if let Some(bounds) = active_bounds {
        let offset = px(30.0);
        let cascaded = Bounds::new(
            point(bounds.origin.x + offset, bounds.origin.y + offset),
            bounds.size,
        );
        WindowBounds::Windowed(cascaded)
    } else {
        persistence::window::load_window_bounds(cx)
    }
}

pub fn open_workspace_window(cx: &mut App, target: WindowLaunchTarget) {
    let window_bounds = match &target {
        WindowLaunchTarget::RestorePersisted => persistence::window::load_window_bounds(cx),
        WindowLaunchTarget::Fresh | WindowLaunchTarget::Session(_) => compute_new_window_bounds(cx),
    };

    let options = WindowOptions {
        window_bounds: Some(window_bounds),
        titlebar: Some(TitlebarOptions {
            title: None,
            appears_transparent: true,
            traffic_light_position: Some(point(px(14.0), px(14.0))),
        }),
        ..Default::default()
    };

    cx.defer(move |cx| {
        let target_clone = target.clone();
        let result = cx.open_window(options, move |window, cx| {
            let app_view = cx.new(|cx| ConsoleDesktopApp::new(window, target_clone, cx));
            let handle = window.window_handle();
            register_window(handle, app_view.downgrade());
            cx.new(|cx| gpui_component::Root::new(app_view, window, cx))
        });

        if let Ok(handle) = result {
            handle
                .update(cx, |_, window, _| {
                    window.activate_window();
                })
                .ok();
        }
    });
}
