use super::*;

impl TranscriptView {
    pub fn has_active_cite_popup(&self) -> bool {
        let state = self.selection.selection.borrow();
        if state.is_empty() || state.is_dragging() {
            return false;
        }
        let Some(head) = state.head_span() else {
            return false;
        };
        if head.key.row.starts_with("user-") {
            return false;
        }
        self.selection.cite_handler().is_some()
    }

    pub fn clear_selection(&mut self, window: &mut Window) {
        self.selection.selection.borrow_mut().clear();
        window.refresh();
    }

    pub fn trigger_cite(&mut self, window: &mut Window, cx: &mut Context<Self>) -> bool {
        if !self.has_active_cite_popup() {
            return false;
        }
        let text = self.selected_text();
        self.clear_selection(window);
        if let Some(text) = text
            && let Some(quote) = crate::markdown::selection::format_cite_quote(&text)
            && let Some(cite) = self.selection.cite_handler()
        {
            cite(quote, window, cx);
            return true;
        }
        false
    }

    pub fn trigger_copy(&mut self, window: &mut Window, cx: &mut Context<Self>) -> bool {
        if !self.has_active_cite_popup() {
            return false;
        }
        let text = self.selected_text();
        self.clear_selection(window);
        if let Some(text) = text {
            cx.write_to_clipboard(gpui::ClipboardItem::new_string(text));
            return true;
        }
        false
    }

    /// The currently selected transcript text, if the user has a selection.
    pub fn selected_text(&self) -> Option<String> {
        self.selection.selection.borrow().selected_text()
    }
}
