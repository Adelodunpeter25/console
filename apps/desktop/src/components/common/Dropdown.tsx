import React from "react";
import { Check, ChevronDown, Search } from "lucide-react";

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
            <div className="max-h-80 overflow-y-auto">{children}</div>
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

/** Search field placed at the top of a dropdown's menu. */
export function DropdownSearch({
  value,
  onChange,
  placeholder = "Search...",
  sticky = true,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  sticky?: boolean;
}) {
  return (
    <div
      className={`${sticky ? "sticky top-0 z-10 bg-card" : ""} relative px-1.5 pb-1`}
    >
      <div className="relative">
        <Search
          size={13}
          className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-foreground-muted"
        />
        <input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          className="h-8 w-full rounded-md border border-white/[0.16] bg-[#1d1d1d] pl-8 pr-2 text-xs text-foreground outline-none placeholder:text-[#737373] transition-colors hover:border-white/[0.22] focus:border-white/[0.28] focus:bg-[#202020] focus:ring-1 focus:ring-white/[0.06]"
        />
      </div>
    </div>
  );
}

/** Non-selecting action row for menu operations such as opening a folder. */
export function DropdownAction({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  const { close } = React.useContext(DropdownContext);
  return (
    <button
      type="button"
      role="menuitem"
      onClick={() => {
        onClick();
        close();
      }}
      className="flex w-full cursor-pointer items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-xs text-foreground-secondary outline-none transition-colors hover:bg-white/[0.07] hover:text-foreground focus-visible:bg-white/[0.08] focus-visible:text-foreground"
    >
      {children}
    </button>
  );
}

export function DropdownSeparator() {
  return <div className="-mx-1 my-1 h-px bg-white/[0.08]" role="separator" />;
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
