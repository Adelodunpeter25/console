import { useEffect, useMemo, useRef } from "react";
import { useValue } from "@legendapp/state/react";
import type { BottomSheetModal } from "@gorhom/bottom-sheet";
import { sessionService } from "@console/api";
import type { TodoItem } from "@console/types";
import { chat$, setTodoItems, getChatSession } from "@/stores/useChatStore";

export function useSessionTodos(sessionId: string | null | undefined) {
  const bottomSheetRef = useRef<BottomSheetModal>(null);

  // Read reactive todo items for the active session from Legend State store
  const todoItems = useValue(() => {
    if (!sessionId) return [] as TodoItem[];
    return chat$.sessions[sessionId]?.todoItems.get() ?? ([] as TodoItem[]);
  });

  // Initial hydration from backend on session mount/change
  useEffect(() => {
    if (!sessionId) return;
    const current = getChatSession(sessionId);
    if (current.todoItems && current.todoItems.length > 0) return;

    let cancelled = false;

    sessionService
      .getTodos(sessionId)
      .then((serverTodos) => {
        if (cancelled) return;
        if (Array.isArray(serverTodos)) {
          const fresh = getChatSession(sessionId);
          if (!fresh.todoItems || fresh.todoItems.length === 0) {
            setTodoItems(sessionId, serverTodos);
          }
        }
      })
      .catch(() => {
        // Silently ignore network errors during initial hydration
      });

    return () => {
      cancelled = true;
    };
  }, [sessionId]);

  const totalCount = todoItems.length;

  const completedCount = useMemo(() => {
    return todoItems.filter(
      (item) =>
        item.status === "completed" ||
        (item.status as string) === "done" ||
        (item.status as string) === "complete",
    ).length;
  }, [todoItems]);

  // True if there is at least 1 task still pending or in progress
  const hasActiveTodos = useMemo(() => {
    return (
      todoItems.length > 0 &&
      todoItems.some(
        (item) =>
          item.status !== "completed" &&
          (item.status as string) !== "done" &&
          (item.status as string) !== "complete",
      )
    );
  }, [todoItems]);

  // Next active task to show in single-line collapsed preview
  const nextPendingTodo = useMemo(() => {
    const inProgress = todoItems.find((t) => t.status === "in_progress");
    if (inProgress) return inProgress;
    return todoItems.find(
      (t) =>
        t.status !== "completed" &&
        (t.status as string) !== "done" &&
        (t.status as string) !== "complete",
    );
  }, [todoItems]);

  const openSheet = () => {
    bottomSheetRef.current?.present();
  };

  const closeSheet = () => {
    bottomSheetRef.current?.dismiss();
  };

  return {
    todoItems,
    totalCount,
    completedCount,
    hasActiveTodos,
    nextPendingTodo,
    bottomSheetRef,
    openSheet,
    closeSheet,
  };
}
