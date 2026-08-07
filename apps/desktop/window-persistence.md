# Window Size Persistence — Review & Fix Plan

**Scope:** `apps/desktop` window size persistence.
**Files involved:**
- `apps/desktop/src/pages/ChatPage.tsx` — restore on mount, debounced persist on `onResized`
- `apps/desktop/src/lib/ui-store.ts` — `LazyStore` (`window.size` key)
- `apps/desktop/src-tauri/tauri.conf.json` — initial window geometry

---

## Current implementation (hand-rolled)

Persist side (`ChatPage.tsx`, ~lines 48–64):

```ts
const win = getCurrentWindow();
let timer: ReturnType<typeof setTimeout> | null = null;
const onResize = async () => {
  const size = await win.innerSize();
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => {
    setWindowSize({ width: size.width, height: size.height }).catch(() => {});
  }, 400);
};
const unlisten = win.onResized(onResize);
```

Restore side (`ChatPage.tsx`, ~lines 29–46):

```ts
const restoreWindow = async () => {
  const size = await getWindowSize();
  if (size) {
    try {
      await getCurrentWindow().setSize(new PhysicalSize(size.width, size.height));
    } catch {
      // Ignore — window may not be resizable in some platforms.
    }
  }
};
restoreWindow();
```

Store (`ui-store.ts`):

```ts
export const uiStore = new LazyStore("ui-preferences.json", { autoSave: false });
export async function getWindowSize() {
  const v = await uiStore.get<{ width: number; height: number }>(UI_KEYS.windowSize);
  return v ?? null;
}
export async function setWindowSize(size) {
  await uiStore.set(UI_KEYS.windowSize, size);
  await uiStore.save();
}
```

---

## Findings

### What works
- Clean separation between store and consumer.
- 400ms debounce prevents store flooding during a drag.
- Careful async cleanup of the `onResized` listener on unmount.
- `autoSave: false` + explicit `save()` avoids write races during fast drags.

### Bug 1 (real): the window does not restore properly — maximized/fullscreen state is lost
When the window is maximized, `innerSize()` returns the *maximized* physical pixel size.
That value is persisted, but the maximized/fullscreen flag is **not** stored. On relaunch:

- `restoreWindow` calls `setSize(...)` with the maximized dimensions while the window is in
  the normal, resizable state → it reopens as a giant non-maximized window.
- Worse, `setSize()` fires `onResized` → the debounced handler immediately re-persists that
  oversized value, locking in the bad state.
- **Net result: the window does not come back reliably to what the user had** — matching the
  reported "it does not restore" symptom.

### Bug 2 (real): no clamping / validation on restore
`getWindowSize()` returns the stored value unchecked (`v ?? null`), and `ChatPage` feeds it
straight into `setSize`. A corrupted entry (missing `height`, `0`, `NaN`, absurdly large)
can produce a broken or off-screen window. The sidebar restore clamps; window size does not.

### Minor issues
1. **Physical vs logical size across monitors / DPI.** `innerSize()` returns a
   `PhysicalSize` (absolute pixels). Across monitors with different scale factors the same
   stored number maps to a different logical appearance. Logical pixels are more portable.
2. **Restore-vs-listener race on mount.** The restore mutation itself triggers `onResized`,
   which re-persists (see Bug 1) — actively writes bad state.
3. **No debounced flush on close.** Writes in the final 400ms before quit are lost.
4. **Silent failure swallowing (`.catch(() => {})`)** hides store errors; add a log.

---

## Decision

**Do not keep hand-rolling. Switch to `tauri-plugin-window-state`.**

It natively persists size, position, **and maximized/fullscreen state**, and restores before
the frontend mounts — solving Bugs 1 & 2 and all the minor issues above at once. This is the
chosen approach for this task.

---

## Migration plan → `tauri-plugin-window-state`

1. **Add crate dependency** (`apps/desktop/src-tauri/Cargo.toml`):

   ```toml
   [dependencies]
   tauri-plugin-window-state = "2"
   ```

2. **Register the plugin** in `apps/desktop/src-tauri/src/lib.rs`:

   ```rust
   tauri::Builder::default()
       .plugin(tauri_plugin_window_state::Builder::default().build())
       // existing plugins + invoke_handler ...
   ```

3. **Add the capability** — window-state exposes `window-state:default` /
   `window-state:allow-*` permissions. Register in
   `apps/desktop/src-tauri/capabilities/*.json`, e.g.:

   ```json
   "permissions": [
     "core:default",
     "window-state:default"
   ]
   ```

4. **Remove the hand-rolled window-size code**:
   - In `ChatPage.tsx`: drop the `restoreWindow` block and the `win.onResized(...)` persist
     effect entirely.
   - In `ui-store.ts`: remove `UI_KEYS.windowSize`, `getWindowSize`, and `setWindowSize`
     (keep sidebar width persistence as-is).

5. **Keep plugin config defaults** (or tune in Cargo code):
   `StateFlags::SIZE | POSITION | MAXIMIZED` etc. are enabled by default. Adjust
   `Builder::default().with_state_flags(...)` if needed.

6. **Verify**: build + run the app, resize, maximise, restart, confirm the window and its
   maximized state come back identical.

---

## Note to self / implementer
- The config's initial geometry in `tauri.conf.json` (1200×800) only applies on a fresh
  launch / first run; the plugin overrides it afterwards.
- Keep sidebar width persistence as-is — it is separate and works.
- If not adopting the plugin, the minimal fix is: persist `isMaximized`, re-apply it after
  `setSize`, validate/clamp dimensions on restore, and avoid re-persisting during restore.