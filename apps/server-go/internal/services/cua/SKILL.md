# Computer use

You were invoked by a `/computer-use <task>` message. The task follows the
command. If it is empty, ask what to drive before acting.

Drive the computer running Console. The `computer` tool runs synchronous
JavaScript with a preinstalled `cua` object; `computer_reset` clears the
session. Methods are supplied by the installed Cua Driver, so check the live
inventory when using a newer capability.

## Calling

```js
var apps = cua.list_apps();
print(apps.content[0].text);
```

Every method waits for its result. Arguments are one object; methods without arguments take none. Results have
`{content, structuredContent, isError}`. Successful action results include a
confirmation when the driver can identify the target. If `isError` is true, read the
structured refusal or message before deciding what to do next. Keep native
window IDs intact. Images are returned as `{type: "image"}` content parts; keep
the image in the final expression so it is shown.

## Safe observe → act → verify loop

1. Discover with `list_apps()` and `list_windows({pid})`.
2. Read the exact target with `get_window_state({pid, window_id, query?, max_elements?, max_depth?, include_screenshot?})`.
3. Prefer the returned `element_id` or fresh `element_token`; never invent row indices.
4. Act with the same exact `{pid, window_id}` and a fresh target.
5. Verify meaningful changes with `verify_state({pid, window_id, expect: [...]})` or a fresh state read. Predicates may use `label_contains` and `value_equals`; an `unknown` verdict means the assertion did not hold or the target was transient, so read state directly instead of retrying.

Element tokens expire when a newer snapshot replaces them. A successful action
only means the driver accepted the call; it is not proof that the intended UI
state was reached. An interrupted or unknown action may have completed: inspect
fresh state before retrying, never replay blindly.

Prefer background delivery when available. Foreground delivery can move the
pointer, take focus, or disrupt the user's active application and must not be
used as an automatic retry. Use coordinates only from a fresh screenshot of
the exact target; do not resize the image or add a window origin.

## Core methods

Observe: `list_apps`, `list_windows`, `get_window_state`, `get_desktop_state`,
`get_screen_size`, `get_cursor_position`, `get_accessibility_tree`,
`verify_state`, `zoom`, and `clipboard_read`.

Act: `click`, `double_click`, `right_click`, `drag`, `type_text`, `set_value`,
`press_key`, `hotkey`, `scroll`, `invoke_menu`, `move_cursor`, and
`clipboard_write`.

Windows/apps: `launch_app`, `bring_to_front`, `set_window_frame`, and
`kill_app` where supported. Launching or terminating applications may have
side effects; only perform what the user requested.

## Browser pages

For Chromium-family page content, use the typed browser workflow rather than
pixel guessing:

```text
get_browser_state({session, pid, window_id})
→ if refused, follow the returned browser_prepare next_call
→ browser_navigate / browser_click / browser_type / browser_pointer
→ get_browser_state again to verify
```

Other browser methods include `browser_dialog`, `browser_set_input_files`, and
`browser_download`. Browser refs and tab IDs are session-scoped and expire;
never guess a tab. Browser setup and existing-profile attachment require
explicit approval. Browser chrome, permission prompts, downloads, and native
file pickers remain native-window tasks.

## Visual perception

If `parse_visual_regions` is advertised, it can describe text and icon regions
from a current driver capture. Use only the same current capture ID for one
pixel action, then capture again after any action, resize, scroll, navigation,
or target change. Never click from an old screenshot. The optional perception
extension is separate from the default driver and may have additional license
requirements.

## Sessions, diagnostics, and recording

Use `start_session` / `end_session` for explicit multi-call workflows when the
driver advertises them. `health_report` and `check_permissions` are read-only
diagnostics. For an explicitly requested trajectory, use
`start_recording` → actions → `stop_recording`; verify the artifacts. Do not
change driver configuration or permissions as hidden setup.

## Rules

- Select the exact target on every action; do not rely on the focused window.
- Keep one controller for a shared desktop; sessions do not isolate UI state.
- User/system permission prompts belong to the user or trusted host.
- Do not alter browser profiles or security settings as hidden setup.
- Refusals are information: follow their suggested next call, and do not
  escalate delivery mode merely because a background route is unavailable.
