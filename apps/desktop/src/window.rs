use std::cell::RefCell;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::SystemTime;
use gpui::{
    AnyWindowHandle, App, AppContext, Bounds, Pixels, TitlebarOptions, WeakEntity, WindowBounds,
    WindowOptions, point, px,
};

use crate::persistence;
use crate::state::ConsoleDesktopApp;

static WINDOW_ID_COUNTER: AtomicU64 = AtomicU64::new(1);

pub fn generate_window_id() -> String {
    let ts = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .map(|d| d.as_millis())
        .unwrap_or(0);
    let count = WINDOW_ID_COUNTER.fetch_add(1, Ordering::Relaxed);
    format!("win-{ts}-{count}")
}

#[derive(Clone, Debug)]
pub enum WindowLaunchTarget {
    RestorePersisted,
    RestoreDescriptor(persistence::PersistedWindowDescriptor),
    Fresh,
    Session(String),
}

thread_local! {
    static LAST_WINDOW_BOUNDS: RefCell<Option<Bounds<Pixels>>> = RefCell::new(None);
    static WORKSPACE_WINDOWS: RefCell<Vec<(AnyWindowHandle, WeakEntity<ConsoleDesktopApp>)>> =
        RefCell::new(Vec::new());
}

pub fn update_active_window_bounds(bounds: Bounds<Pixels>) {
    LAST_WINDOW_BOUNDS.with(|b| {
        b.replace(Some(bounds));
    });
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
        let live_handles = cx.windows();
        list.retain(|(handle, app)| {
            app.upgrade().is_some() && live_handles.contains(handle)
        });

        // 1. Ask GPUI's platform layer which window is currently focused.
        if let Some(active_handle) = cx.active_window() {
            if let Some((_, app)) = list.iter().find(|(h, _)| *h == active_handle) {
                if let Some(upgraded) = app.upgrade() {
                    return Some((active_handle, upgraded));
                }
            }
        }

        // 2. Fallback to the most recent known window in our list.
        for (handle, app) in list.iter().rev() {
            if let Some(upgraded) = app.upgrade() {
                return Some((*handle, upgraded));
            }
        }

        None
    })
}

pub fn compute_new_window_bounds(cx: &mut App) -> WindowBounds {
    let current_bounds = LAST_WINDOW_BOUNDS
        .with(|b| *b.borrow())
        .unwrap_or_else(|| match persistence::window::load_window_bounds(cx) {
            WindowBounds::Windowed(b) | WindowBounds::Maximized(b) => b,
            WindowBounds::Fullscreen(b) => b,
        });

    let offset = px(30.0);
    let cascaded = Bounds::new(
        point(current_bounds.origin.x + offset, current_bounds.origin.y + offset),
        current_bounds.size,
    );

    update_active_window_bounds(cascaded);
    WindowBounds::Windowed(cascaded)
}

pub fn open_workspace_window(cx: &mut App, target: WindowLaunchTarget) {
    let window_bounds = match &target {
        WindowLaunchTarget::RestoreDescriptor(desc) => {
            let bounds = desc.bounds.bounds();
            update_active_window_bounds(bounds);
            if desc.bounds.maximized {
                WindowBounds::Maximized(bounds)
            } else {
                WindowBounds::Windowed(bounds)
            }
        }
        WindowLaunchTarget::RestorePersisted => {
            let b = persistence::window::load_window_bounds(cx);
            let bounds = match b {
                WindowBounds::Windowed(bounds) | WindowBounds::Maximized(bounds) => bounds,
                WindowBounds::Fullscreen(bounds) => bounds,
            };
            update_active_window_bounds(bounds);
            b
        }
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

    let is_persisted = matches!(
        target,
        WindowLaunchTarget::RestorePersisted | WindowLaunchTarget::RestoreDescriptor(_)
    );
    let open = move |cx: &mut App| {
        let target_clone = target.clone();
        let result = cx.open_window(options, move |window, cx| {
            let app_view = cx.new(|cx| ConsoleDesktopApp::new(window, target_clone, cx));
            let handle = window.window_handle();
            register_window(handle, app_view.downgrade());
            cx.new(|cx| gpui_component::Root::new(app_view, window, cx))
        });

        match result {
            Ok(handle) => {
                handle
                    .update(cx, |_, window, _| {
                        window.activate_window();
                    })
                    .ok();
            }
            Err(err) => {
                log::error!("Failed to open workspace window: {err:?}");
            }
        }
    };

    if is_persisted {
        open(cx);
    } else {
        cx.defer(open);
    }
}
