//! Updates settings page: check for, download and install a newer build.

use crate::theme::Theme;
use gpui::prelude::FluentBuilder;
use gpui::{
    App, FontWeight, InteractiveElement, IntoElement, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::rc::Rc;

#[derive(Clone, Debug, Default, PartialEq)]
pub enum UpdateStatus {
    #[default]
    Idle,
    Checking,
    UpToDate,
    Available {
        build: u64,
    },
    Downloading(f32),
    Failed(String),
}

#[derive(IntoElement)]
pub struct UpdatesPage {
    /// None for a local build, which has no number to compare.
    pub installed_build: Option<u64>,
    pub status: UpdateStatus,
    pub on_check: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_install: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for UpdatesPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let installed = match self.installed_build {
            Some(build) => format!("Installed build {build}"),
            None => "Local build".to_string(),
        };
        let summary = match &self.status {
            UpdateStatus::Idle => installed.clone(),
            UpdateStatus::Checking => "Checking for updates…".to_string(),
            UpdateStatus::UpToDate => format!("You're on the latest version. {installed}."),
            UpdateStatus::Available { build } => {
                format!("Build {build} is available. {installed}.")
            }
            UpdateStatus::Downloading(progress) => {
                format!("Downloading update… {}%", (progress * 100.0) as u32)
            }
            UpdateStatus::Failed(message) => message.clone(),
        };
        let failed = matches!(self.status, UpdateStatus::Failed(_));
        let busy = matches!(
            self.status,
            UpdateStatus::Checking | UpdateStatus::Downloading(_)
        );
        let available = matches!(self.status, UpdateStatus::Available { .. });
        let disabled = busy || self.installed_build.is_none();
        let on_click = if available {
            self.on_install
        } else {
            self.on_check
        };

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(16.0))
                            .font_weight(FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child("Updates"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .text_color(theme.text_secondary)
                            .child("Console restarts to finish installing an update."),
                    ),
            )
            .child(
                div()
                    .p(px(12.0))
                    .rounded(px(8.0))
                    .border_1()
                    .border_color(theme.border)
                    .bg(theme.surface)
                    .text_size(px(12.5))
                    .text_color(if failed {
                        theme.danger
                    } else {
                        theme.text_secondary
                    })
                    .child(summary),
            )
            .child(
                div().flex().child(
                    div()
                        .id("updates-action")
                        .when(disabled, |el| el.opacity(0.4))
                        .cursor_pointer()
                        .rounded(px(6.0))
                        .bg(theme.accent)
                        .px(px(12.0))
                        .py(px(6.0))
                        .text_size(px(12.0))
                        .font_weight(FontWeight::MEDIUM)
                        .text_color(theme.on_inverse)
                        .when(!disabled, |el| el.hover(|s| s.opacity(0.9)))
                        .child(if available {
                            "Install and restart"
                        } else {
                            "Check for updates"
                        })
                        .on_click(move |_event, window, cx| {
                            if !disabled {
                                (on_click)(window, cx);
                            }
                        }),
                ),
            )
    }
}
