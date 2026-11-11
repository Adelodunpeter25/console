use super::*;

impl TranscriptView {
    /// Wire clicks on a turn's changed-file chip (the app opens the diff tab).
    pub fn set_on_open_change(
        &mut self,
        handler: impl Fn(String, u32, Option<String>, &mut Window, &mut App) + 'static,
    ) {
        self.on_open_change = Some(Rc::new(handler));
    }

    /// Wire lazy diff loading for the chip hover preview.
    pub fn set_load_diff(
        &mut self,
        handler: impl Fn(&str, u32, &mut App) -> gpui::Task<Option<String>> + 'static,
    ) {
        self.load_diff = Some(Rc::new(handler));
    }

    /// Wire assistant-message fork clicks (the app forks the session through
    /// the clicked message and opens the new chat).
    pub fn set_on_fork(&mut self, handler: impl Fn(String, &mut Window, &mut App) + 'static) {
        self.on_fork = Some(Rc::new(handler));
    }

    /// Wire the Cite popup: the app inserts the quoted selection into the
    /// pane's composer. The handler lives on the shared selection state so
    /// the selection mouse handlers can invoke it directly; the popup only
    /// renders when one is wired.
    pub fn set_on_cite(&mut self, handler: impl Fn(String, &mut Window, &mut App) + 'static) {
        self.selection.set_cite_handler(Some(Rc::new(handler)));
    }

    /// Wire the image-preview opener (the app opens its modal with the
    /// decoded image). Called by the shell when rendering the transcript.
    pub fn set_on_preview_image(
        &mut self,
        handler: impl Fn(Arc<gpui::Image>, &mut Window, &mut App) + 'static,
    ) {
        self.on_preview_image = Some(Rc::new(handler));
    }

    pub fn set_on_view_subagent(
        &mut self,
        handler: impl Fn(String, &mut Window, &mut App) + 'static,
    ) {
        self.on_view_subagent = Some(Rc::new(handler));
    }

    /// Wire file-path link clicks (the app opens a workspace tab). Same shape
    /// as `on_preview_image`: the transcript stays unaware of projects and
    /// panes, the shell owns resolution + opening. File-like destinations
    /// forward their raw link text; `http(s)` goes to `on_open_url_raw` when
    /// set, otherwise the external browser.
    pub fn set_on_open_file(&mut self, handler: impl Fn(String, &mut Window, &mut App) + 'static) {
        self.on_open_file_raw = Some(Rc::new(handler));
        self.rebuild_link_handler();
    }

    /// Wire web-link clicks (the app opens its embedded browser). When unset,
    /// `http(s)` falls back to the external browser.
    pub fn set_on_open_url(&mut self, handler: impl Fn(String, &mut Window, &mut App) + 'static) {
        self.on_open_url_raw = Some(Rc::new(handler));
        self.rebuild_link_handler();
    }

    pub(super) fn rebuild_link_handler(&mut self) {
        let open_file = self.on_open_file_raw.clone();
        let open_url = self.on_open_url_raw.clone();
        self.link_handler = Some(Rc::new(
            move |url: &str, window: &mut Window, cx: &mut App| {
                if is_http_url(url) {
                    if let Some(open_url) = &open_url {
                        open_url(url.to_owned(), window, cx);
                    } else {
                        cx.open_url(url);
                    }
                } else if is_file_link(url) {
                    if let Some(open_file) = &open_file {
                        open_file(url.to_owned(), window, cx);
                    }
                } else {
                    cx.open_url(url);
                }
            },
        ));
    }

    pub fn set_on_load_older(&mut self, handler: impl Fn(&mut Window, &mut App) + 'static) {
        self.on_load_older = Some(Rc::new(handler));
    }

    /// Fetches the chat's full prompt list when the popup opens.
    pub fn set_on_request_prompts(&mut self, handler: impl Fn(&mut Window, &mut App) + 'static) {
        self.on_request_prompts = Some(Rc::new(handler));
    }

    /// Jumps to a prompt, given as its position counted from the newest (0 =
    /// newest), paging older history in if needed.
    pub fn set_on_jump_to_prompt(
        &mut self,
        handler: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) {
        self.on_jump_to_prompt = Some(Rc::new(handler));
    }

    pub(crate) fn jump_callback(
        &self,
    ) -> Option<Rc<dyn Fn(usize, &mut Window, &mut App) + 'static>> {
        self.on_jump_to_prompt.clone()
    }
}
