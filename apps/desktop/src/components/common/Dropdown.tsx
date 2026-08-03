import React from "react";
import { ChevronDown, Check } from "lucide-react";

/* ------------------------------------------------------------------ */
/* Reusable dropdown primitives — shared visual language for           */
/* model selector, approval mode selector, etc.                        */
/* ------------------------------------------------------------------ */

interface DropdownContextValue {
  close: () => void;
}

const DropdownContext = React.createContext<DropdownContextValue>({ close: () => {} });

interface DropdownProps {
  /** Currently selected value label shown in the trigger button. */
  label: string;
  /** Section heading shown at the top of the popover. */
  heading: string;
  /** Called when the trigger is clicked. Use to lazy-load options. */
  onOpen?: () => void;
  /** Popover width in px. */
  width?: number;
  children: React.ReactNode;
}

/**
 * Generic dropdown with the same visual style as the model selector:
 * compact trigger button, upward-opening popover with a heading.
 * Closes automatically when any DropdownItem is selected.
 */
export function Dropdown({ label, heading, onOpen, width = 256, children }: DropdownProps) {
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const handleToggle = () => {
    if (!open) onOpen?.();
    setOpen(!open);
  };

  const close = () => setOpen(false);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={handleToggle}
        className="flex items-center gap-1.5 rounded-md px-1.5 py-1 text-xs font-medium text-foreground-secondary outline-none transition-colors hover:bg-white/[0.06] hover:text-foreground focus-visible:bg-white/[0.08] focus-visible:text-foreground"
      >
        {label}
        <ChevronDown
          size={13}
          className={`text-foreground-muted transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>

      {open && (
        <DropdownContext.Provider value={{ close }}>
          <div
            role="menu"
            className="absolute bottom-full left-0 z-50 mb-1.5 max-h-[var(--available-height)] min-w-32 origin-bottom-left overflow-x-hidden overflow-y-auto rounded-lg border border-white/[0.1] bg-card p-1 text-foreground shadow-lg ring-1 ring-foreground/10"
            style={{ width }}
          >
            <div className="px-1.5 py-1 text-[10px] font-medium uppercase tracking-wider text-foreground-muted">
              <span>
                {heading}
              </span>
            </div>
            <div>{children}</div>
          </div>
        </DropdownContext.Provider>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Dropdown item — check-marked selectable row                          */
/* ------------------------------------------------------------------ */

interface DropdownItemProps {
  selected: boolean;
  onClick: () => void;
  children: React.ReactNode;
}

export function DropdownItem({ selected, onClick, children }: DropdownItemProps) {
  const { close } = React.useContext(DropdownContext);
  return (
    <button
      type="button"
      role="menuitem"
      onClick={() => {
        onClick();
        close();
      }}
      className="group relative flex w-full cursor-pointer items-center gap-1.5 rounded-md px-1.5 py-1.5 pr-8 text-left text-xs text-foreground-secondary outline-none transition-colors hover:bg-white/[0.07] hover:text-foreground focus-visible:bg-white/[0.08] focus-visible:text-foreground"
    >
      <span className="min-w-0 flex-1 truncate">{children}</span>
      <span className="pointer-events-none absolute right-2 flex items-center justify-center text-success">
        {selected && <Check size={13} />}
      </span>
    </button>
  );
}

/* ------------------------------------------------------------------ */
/* Dropdown group heading — for sub-sections inside the popover         */
/* ------------------------------------------------------------------ */

export function DropdownGroupHeading({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-1.5 pb-1 pt-2 text-[10px] font-medium uppercase tracking-wider text-foreground-muted first:pt-1">
      {children}
    </div>
  );
}
