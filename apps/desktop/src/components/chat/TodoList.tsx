import React from "react";
import { Check, ChevronDown, Circle } from "lucide-react";
import type { TodoItem } from "@console/types";
import { MarkdownRenderer } from "../common/MarkdownRenderer";

interface TodoListProps {
  items: TodoItem[];
}

export function TodoList({ items }: TodoListProps) {
  const [collapsed, setCollapsed] = React.useState(true);
  const nextIndex = items.findIndex((item) => item.status !== "completed");
  if (nextIndex === -1) return null;

  const visibleItems = collapsed ? [items[nextIndex]!] : items;

  return (
    <section className="rounded-lg border border-white/[0.08] bg-white/[0.025] px-3 py-2">
      <div className="mb-1.5 flex items-center justify-between gap-3">
        <span className="text-[11px] font-medium uppercase tracking-wide text-foreground-muted">
          Todos
        </span>
        <button
          type="button"
          aria-label={collapsed ? "Expand todos" : "Collapse todos"}
          aria-expanded={!collapsed}
          onClick={() => setCollapsed((current) => !current)}
          className="rounded p-0.5 text-foreground-muted transition-colors hover:bg-white/[0.08] hover:text-foreground"
        >
          <ChevronDown size={15} className={collapsed ? "rotate-180" : ""} />
        </button>
      </div>
      <div className="space-y-1">
        {visibleItems.map((item) => {
          const completed = item.status === "completed";
          return (
            <div key={item.id} className="flex items-start gap-2 text-sm text-foreground-secondary">
              <span className="mt-0.5 shrink-0" aria-hidden="true">
                {completed ? (
                  <span className="flex h-4 w-4 items-center justify-center rounded-full bg-success text-screen">
                    <Check size={11} strokeWidth={3} />
                  </span>
                ) : item.status === "in_progress" ? (
                  <span className="flex h-4 w-4 items-center justify-center rounded-full border border-blue-400">
                    <span className="h-1.5 w-1.5 rounded-full bg-blue-400" />
                  </span>
                ) : (
                  <Circle size={16} className="text-foreground-muted" />
                )}
              </span>
              <div className={completed ? "text-foreground-muted line-through" : undefined}>
                <MarkdownRenderer content={item.content} />
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
