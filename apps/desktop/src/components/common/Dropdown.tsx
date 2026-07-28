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
        onClick={handleToggle}
        className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
      >
        {label}
        <ChevronDown size={14} className={`transition-transform ${open ? "rotate-180" : ""}`} />
      </button>

      {open && (
        <DropdownContext.Provider value={{ close }}>
          <div
            className="absolute bottom-full left-0 mb-2 bg-card border border-border-strong rounded-xl shadow-2xl overflow-hidden z-50"
            style={{ width }}
          >
            <div className="px-3 py-2 border-b border-border">
              <span className="text-xs font-semibold text-foreground-muted uppercase tracking-wider">
                {heading}
              </span>
            </div>
            <div className="max-h-64 overflow-y-auto">{children}</div>
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
      onClick={() => {
        onClick();
        close();
      }}
      className="w-full flex items-center justify-between px-3 py-2 text-sm text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors text-left"
    >
      <span className="truncate font-mono text-xs">{children}</span>
      {selected && <Check size={14} className="text-success shrink-0" />}
    </button>
  );
}

/* ------------------------------------------------------------------ */
/* Dropdown group heading — for sub-sections inside the popover         */
/* ------------------------------------------------------------------ */

export function DropdownGroupHeading({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-3 py-1.5 text-xs font-semibold text-foreground-muted uppercase tracking-wider bg-white/[0.02]">
      {children}
    </div>
  );
}
