//! Error banner surfacing and the Cmd+C fallback that copies transcript or
//! banner selections.

use console_ui::CopySelection;
use gpui::{ClipboardItem, Context, Window};

use super::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Surface an error banner. It auto-dismisses after a few seconds (or
    /// sooner if the user copies it or clears it manually); a newer error
    /// replaces any pending dismissal.
    pub fn set_error(&mut self, message: impl Into<String>, cx: &mut Context<Self>) {
        self.error_message = Some(message.into());
        self.error_generation = self.error_generation.wrapping_add(1);
        let generation = self.error_generation;
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            cx.background_executor()
                .timer(std::time::Duration::from_secs(5))
                .await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        if this.error_generation == generation {
                            this.clear_error(cx);
                        }
                    });
                }
            });
        })
        .detach();
        cx.notify();
    }

    pub fn clear_error(&mut self, cx: &mut Context<Self>) {
        if self.error_message.take().is_some() {
            self.error_selection.clear();
            cx.notify();
        }
    }

    /// Fallback leg of the Cmd+C shortcut: the composer answers the keystroke
    /// first and propagates when it has nothing selected of its own; here the
    /// transcript's text selection answers instead.
    pub fn copy_selection_action(
        &mut self,
        _: &CopySelection,
        _: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let selected = self
            .error_selection
            .selection
            .borrow()
            .selected_text()
            .or_else(|| self.active_transcript_view().read(cx).selected_text());
        match selected {
            Some(text) => cx.write_to_clipboard(ClipboardItem::new_string(text)),
            None => cx.propagate(),
        }
    }
}
