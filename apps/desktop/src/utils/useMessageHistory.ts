import React from "react";

interface UseMessageHistoryOptions {
  history: string[];
  value: string;
  onChange: (value: string) => void;
}

/** Provides shell-style Up/Down navigation through previous chat prompts. */
export function useMessageHistory({
  history,
  value,
  onChange,
}: UseMessageHistoryOptions) {
  const indexRef = React.useRef<number | null>(null);
  const draftRef = React.useRef("");

  const reset = React.useCallback(() => {
    indexRef.current = null;
  }, []);

  const navigate = React.useCallback(
    (direction: -1 | 1, event: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (history.length === 0) return;
      const cursor = event.currentTarget.selectionStart;
      if (direction < 0 && cursor !== 0) return;
      if (direction > 0 && cursor !== value.length) return;

      event.preventDefault();
      if (direction < 0) {
        if (indexRef.current === null) draftRef.current = value;
        const next =
          indexRef.current === null
            ? history.length - 1
            : Math.max(0, indexRef.current - 1);
        indexRef.current = next;
        onChange(history[next] ?? "");
        return;
      }

      if (indexRef.current === null) return;
      const next = indexRef.current + 1;
      if (next >= history.length) {
        indexRef.current = null;
        onChange(draftRef.current);
      } else {
        indexRef.current = next;
        onChange(history[next] ?? "");
      }
    },
    [history, onChange, value],
  );

  return { navigate, reset };
}
