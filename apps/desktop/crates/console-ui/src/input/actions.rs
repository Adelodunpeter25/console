use gpui::{App, KeyBinding, actions};

actions!(
    composer,
    [
        Backspace,
        Delete,
        Left,
        Right,
        Up,
        Down,
        SelectLeft,
        SelectRight,
        SelectAll,
        Home,
        End,
        MoveToPreviousWord,
        MoveToNextWord,
        SelectToStart,
        SelectToEnd,
        SelectToPreviousWord,
        SelectToNextWord,
        DeleteToStart,
        DeleteToEnd,
        DeleteToPreviousWord,
        DeleteToNextWord,
        Paste,
        Cut,
        Copy,
        Undo,
        Redo,
        Enter,
        Newline,
        SubmitSteer,
        // Fallback leg of the copy shortcut: the composer handles Cmd+C
        // first and propagates when it has nothing selected, letting the
        // transcript's text selection answer the keystroke.
        CopySelection,
    ]
);

pub fn init_input_keybindings(cx: &mut App) {
    cx.bind_keys([
        KeyBinding::new("backspace", Backspace, Some("ComposerInput")),
        KeyBinding::new("delete", Delete, Some("ComposerInput")),
        KeyBinding::new("alt-backspace", DeleteToPreviousWord, Some("ComposerInput")),
        KeyBinding::new("alt-delete", DeleteToNextWord, Some("ComposerInput")),
        KeyBinding::new("left", Left, Some("ComposerInput")),
        KeyBinding::new("right", Right, Some("ComposerInput")),
        KeyBinding::new("up", Up, Some("ComposerInput")),
        KeyBinding::new("down", Down, Some("ComposerInput")),
        KeyBinding::new("shift-left", SelectLeft, Some("ComposerInput")),
        KeyBinding::new("shift-right", SelectRight, Some("ComposerInput")),
        KeyBinding::new("home", Home, Some("ComposerInput")),
        KeyBinding::new("end", End, Some("ComposerInput")),
        KeyBinding::new("shift-home", SelectToStart, Some("ComposerInput")),
        KeyBinding::new("shift-end", SelectToEnd, Some("ComposerInput")),
        KeyBinding::new("alt-left", MoveToPreviousWord, Some("ComposerInput")),
        KeyBinding::new("alt-right", MoveToNextWord, Some("ComposerInput")),
        KeyBinding::new(
            "alt-shift-left",
            SelectToPreviousWord,
            Some("ComposerInput"),
        ),
        KeyBinding::new("alt-shift-right", SelectToNextWord, Some("ComposerInput")),
        KeyBinding::new("secondary-a", SelectAll, Some("ComposerInput")),
        KeyBinding::new("secondary-v", Paste, Some("ComposerInput")),
        KeyBinding::new("secondary-c", Copy, Some("ComposerInput")),
        KeyBinding::new("secondary-x", Cut, Some("ComposerInput")),
        // Global fallback: answers Cmd+C whenever a deeper binding (the
        // composer) propagated because it had nothing to copy.
        KeyBinding::new("secondary-c", CopySelection, None),
        KeyBinding::new("secondary-z", Undo, Some("ComposerInput")),
        KeyBinding::new("secondary-shift-z", Redo, Some("ComposerInput")),
        KeyBinding::new("enter", Enter, Some("ComposerInput")),
        KeyBinding::new("shift-enter", Newline, Some("ComposerInput")),
        // While a turn is running, Enter queues a follow-up; the platform's
        // primary modifier + Enter injects it when the provider supports it.
        KeyBinding::new("secondary-enter", SubmitSteer, Some("ComposerInput")),
    ]);

    #[cfg(target_os = "macos")]
    cx.bind_keys([
        KeyBinding::new("cmd-backspace", DeleteToStart, Some("ComposerInput")),
        KeyBinding::new("cmd-delete", DeleteToEnd, Some("ComposerInput")),
        KeyBinding::new("ctrl-h", Backspace, Some("ComposerInput")),
        KeyBinding::new("ctrl-d", Delete, Some("ComposerInput")),
        KeyBinding::new("ctrl-u", DeleteToStart, Some("ComposerInput")),
        KeyBinding::new("ctrl-k", DeleteToEnd, Some("ComposerInput")),
        KeyBinding::new("ctrl-b", Left, Some("ComposerInput")),
        KeyBinding::new("ctrl-f", Right, Some("ComposerInput")),
        KeyBinding::new("cmd-left", Home, Some("ComposerInput")),
        KeyBinding::new("cmd-right", End, Some("ComposerInput")),
        KeyBinding::new("cmd-up", Home, Some("ComposerInput")),
        KeyBinding::new("cmd-down", End, Some("ComposerInput")),
        KeyBinding::new("ctrl-a", Home, Some("ComposerInput")),
        KeyBinding::new("ctrl-e", End, Some("ComposerInput")),
        KeyBinding::new("shift-cmd-left", SelectToStart, Some("ComposerInput")),
        KeyBinding::new("shift-cmd-right", SelectToEnd, Some("ComposerInput")),
        KeyBinding::new("cmd-shift-up", SelectToStart, Some("ComposerInput")),
        KeyBinding::new("cmd-shift-down", SelectToEnd, Some("ComposerInput")),
        KeyBinding::new("ctrl-shift-a", SelectToStart, Some("ComposerInput")),
        KeyBinding::new("ctrl-shift-e", SelectToEnd, Some("ComposerInput")),
    ]);

    #[cfg(not(target_os = "macos"))]
    cx.bind_keys([
        KeyBinding::new(
            "ctrl-backspace",
            DeleteToPreviousWord,
            Some("ComposerInput"),
        ),
        KeyBinding::new("ctrl-delete", DeleteToNextWord, Some("ComposerInput")),
        KeyBinding::new("ctrl-left", MoveToPreviousWord, Some("ComposerInput")),
        KeyBinding::new("ctrl-right", MoveToNextWord, Some("ComposerInput")),
        KeyBinding::new(
            "ctrl-shift-left",
            SelectToPreviousWord,
            Some("ComposerInput"),
        ),
        KeyBinding::new("ctrl-shift-right", SelectToNextWord, Some("ComposerInput")),
    ]);
}
