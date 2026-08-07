import React from "react";
import { createPortal } from "react-dom";

/* ------------------------------------------------------------------ */
/* Context menu — right-click menu rendered at the cursor position.    */
/*                                                                     */
/* Usage:                                                              */
/*   <ContextMenuProvider>       // mount once, near the app root      */
/*     <App />                                                        */
/*   </ContextMenuProvider>                                           */
/*                                                                     */
/*   const menu = useContextMenu();                                   */
/*   <div onContextMenu={(e) => {                                     */
/*     e.preventDefault();                                            */
/*     menu.open(e.clientX, e.clientY, [                             */
/*       { label: "Rename", icon: <Pencil size={13} />, onClick: ... },*/
/*       { label: "Delete", danger: true, separatorBefore: true,       */
/*         onClick: ... },                                             */
/*     ]);                                                             */
/*   }}>                                                              */
/*                                                                     */
/* Rendered through a portal so overflow/clip ancestors cannot cut     */
/* it off. Dismisses on outside mousedown, Escape, scroll, and blur.   */
/* ------------------------------------------------------------------ */

export interface ContextMenuItem {
  label: string;
  icon?: React.ReactNode;
  danger?: boolean;
  disabled?: boolean;
  separatorBefore?: boolean;
  onClick: () => void;
}

interface ContextMenuState {
  x: number;
  y: number;
  items: ContextMenuItem[];
}

interface ContextMenuContextValue {
  open: (x: number, y: number, items: ContextMenuItem[]) => void;
  close: () => void;
}

const ContextMenuContext = React.createContext<ContextMenuContextValue>({
  open: () => {},
  close: () => {},
});

/** Hook to open/close the context menu from any component. */
export function useContextMenu(): ContextMenuContextValue {
  return React.useContext(ContextMenuContext);
}

const MENU_PADDING = 8;
const MENU_WIDTH = 200;
const MENU_MAX_HEIGHT = 320;

/** Render the active context menu at the cursor, clamped to the viewport. */
function ContextMenuPortal({ state, onClose }: { state: ContextMenuState; onClose: () => void }) {
  const ref = React.useRef<HTMLDivElement>(null);
  const [position, setPosition] = React.useState({ x: state.x, y: state.y });
  const [focusedIndex, setFocusedIndex] = React.useState(0);
  const itemRefs = React.useRef<Array<HTMLButtonElement | null>>([]);

  // Clamp the menu inside the viewport once its size is known.
  React.useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const maxX = window.innerWidth - rect.width - MENU_PADDING;
    const maxY = window.innerHeight - rect.height - MENU_PADDING;
    setPosition({
      x: Math.min(state.x, Math.max(MENU_PADDING, maxX)),
      y: Math.min(state.y, Math.max(MENU_PADDING, maxY)),
    });
  }, [state.x, state.y]);

  // Focus the first enabled item on open.
  React.useEffect(() => {
    itemRefs.current[0]?.focus();
  }, []);

  // Escape closes; arrows move focus between enabled items.
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const direction = event.key === "ArrowDown" ? 1 : -1;
      const enabled = state.items
        .map((item, i) => ({ item, i }))
        .filter(({ item }) => !item.disabled);
      if (enabled.length === 0) return;
      const currentPos = enabled.findIndex(({ i }) => i === focusedIndex);
      const next = enabled[(currentPos + direction + enabled.length) % enabled.length]!;
      setFocusedIndex(next.i);
      itemRefs.current[next.i]?.focus();
    }
  };

  return createPortal(
    <div
      ref={ref}
      role="menu"
      onKeyDown={handleKeyDown}
      className="fixed z-[100] rounded-lg border border-white/[0.1] bg-card p-1 text-foreground shadow-xl ring-1 ring-foreground/10 outline-none"
      style={{
        left: position.x,
        top: position.y,
        width: MENU_WIDTH,
        maxHeight: MENU_MAX_HEIGHT,
        overflowY: "auto",
      }}
    >
      {state.items.map((item, index) => {
        const isLastSeparator =
          item.separatorBefore &&
          index === 0; // separator before the first item is pointless
        return (
          <React.Fragment key={`${index}-${item.label}`}>
            {item.separatorBefore && !isLastSeparator && (
              <div className="-mx-1 my-1 h-px bg-white/[0.08]" role="separator" />
            )}
            <button
              type="button"
              role="menuitem"
              ref={(el) => {
                itemRefs.current[index] = el;
              }}
              disabled={item.disabled}
              onClick={() => {
                if (item.disabled) return;
                onClose();
                item.onClick();
              }}
              className={`flex w-full cursor-pointer items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-xs outline-none transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
                item.danger
                  ? "text-danger hover:bg-danger/10 hover:text-danger focus-visible:bg-danger/10 focus-visible:text-danger"
                  : "text-foreground-secondary hover:bg-white/[0.07] hover:text-foreground focus-visible:bg-white/[0.08] focus-visible:text-foreground"
              }`}
            >
              {item.icon && (
                <span className="shrink-0 text-foreground-muted">{item.icon}</span>
              )}
              <span className="min-w-0 flex-1 truncate">{item.label}</span>
            </button>
          </React.Fragment>
        );
      })}
    </div>,
    document.body,
  );
}

/**
 * Provider that renders the active context menu through a portal.
 * Mount once at the app root; surfaces call useContextMenu().open(...).
 */
export function ContextMenuProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = React.useState<ContextMenuState | null>(null);

  const open = React.useCallback((x: number, y: number, items: ContextMenuItem[]) => {
    setState({ x, y, items });
  }, []);

  const close = React.useCallback(() => setState(null), []);

  // Dismiss on outside mousedown, scroll (capture so inner scrollbars still
  // close it), and window blur.
  React.useEffect(() => {
    if (!state) return;

    function onMouseDown(event: MouseEvent) {
      const el = event.target as Node;
      // The portal menu is the only element we should keep open for.
      if (!(el instanceof Element) || !el.closest("[role='menu']")) {
        close();
      }
    }
    function onScroll() {
      close();
    }
    function onBlur() {
      close();
    }

    document.addEventListener("mousedown", onMouseDown);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("blur", onBlur);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("blur", onBlur);
    };
  }, [state, close]);

  return (
    <ContextMenuContext.Provider value={{ open, close }}>
      {children}
      {state && <ContextMenuPortal state={state} onClose={close} />}
    </ContextMenuContext.Provider>
  );
}
