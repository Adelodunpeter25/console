import React from "react";
import { Search, SquarePen, X } from "lucide-react";

interface SearchBarProps {
  value: string;
  onChange: (value: string) => void;
  onNewChat?: () => void;
}

/**
 * Sidebar Search Bar component.
 * Features search icon, input field, clear (X) icon when typing,
 * and quick new chat action button.
 */
export function SearchBar({ value, onChange, onNewChat }: SearchBarProps) {
  return (
    <div className="px-3 pt-3 pb-2 flex items-center gap-2 shrink-0">
      <div className="flex-1 flex items-center gap-2 bg-white/[0.05] border border-white/10 rounded-lg px-2.5 py-1.5">
        <Search size={14} className="text-foreground-muted shrink-0" />
        <input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="Search..."
          className="w-full bg-transparent text-xs text-foreground placeholder:text-foreground-muted outline-none"
        />
        {value.length > 0 && (
          <button
            onClick={() => onChange("")}
            className="text-foreground-muted hover:text-foreground shrink-0 cursor-pointer"
            title="Clear search"
          >
            <X size={14} />
          </button>
        )}
      </div>

      {onNewChat && (
        <button
          onClick={onNewChat}
          className="p-1.5 text-foreground-muted hover:text-foreground rounded-lg shrink-0 cursor-pointer"
          title="New Chat"
        >
          <SquarePen size={16} />
        </button>
      )}
    </div>
  );
}
