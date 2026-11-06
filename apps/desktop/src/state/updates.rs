//! In-app updates (Updates settings tab): version check, download, and the
//! hand-off to the installer, which restarts the app.

use console_core::services::update;
use console_ui::settings::UpdateStatus;
use gpui::Context;

use crate::state::app::ConsoleDesktopApp;

/// CI's run number, baked in at build time. None for local builds.
pub fn installed_build() -> Option<u64> {
    option_env!("CONSOLE_BUILD_NUMBER").and_then(|n| n.parse().ok())
}

impl ConsoleDesktopApp {
    pub fn check_for_update(&mut self, cx: &mut Context<Self>) {
        let Some(installed) = installed_build() else { return };
        if matches!(self.update_status, UpdateStatus::Checking | UpdateStatus::Downloading(_)) {
            return;
        }
        self.update_status = UpdateStatus::Checking;
        cx.notify();

        cx.spawn(async move |entity, cx| {
            let result = update::check(installed).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.update_status = match result {
                            Ok(Some(manifest)) => UpdateStatus::Available { build: manifest.build },
                            Ok(None) => UpdateStatus::UpToDate,
                            Err(error) => UpdateStatus::Failed(format!("{error:#}")),
                        };
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Downloads the update, then quits so the helper can swap it in.
    pub fn install_update(&mut self, cx: &mut Context<Self>) {
        if !matches!(self.update_status, UpdateStatus::Available { .. }) {
            return;
        }
        self.update_status = UpdateStatus::Downloading(0.0);
        cx.notify();

        cx.spawn(async move |entity, cx| {
            let progress_entity = entity.clone();
            let progress_cx = cx.clone();
            let result = update::download(move |progress| {
                let _ = progress_cx.update(|cx| {
                    if let Some(app) = progress_entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.update_status = UpdateStatus::Downloading(progress);
                            cx.notify();
                        });
                    }
                });
            })
            .await
            .and_then(|zip| update::install_and_relaunch(&zip));
            let _ = cx.update(|cx| match result {
                Ok(()) => cx.quit(),
                Err(error) => {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.update_status = UpdateStatus::Failed(format!("{error:#}"));
                            cx.notify();
                        });
                    }
                }
            });
        })
        .detach();
    }
}
