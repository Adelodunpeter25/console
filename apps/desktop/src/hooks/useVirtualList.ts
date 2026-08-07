import React from "react";
import { useVirtualizer, Virtualizer } from "@tanstack/react-virtual";

interface UseVirtualListOptions<T> {
  items: T[];
  estimateSize?: number;
  overscan?: number;
}

interface UseVirtualListResult<T> {
  parentRef: React.RefObject<HTMLDivElement | null>;
  virtualizer: Virtualizer<HTMLDivElement, Element>;
  virtualItems: ReturnType<Virtualizer<HTMLDivElement, Element>["getVirtualItems"]>;
  totalSize: number;
}

/**
 * Centralized virtualization hook wrapping @tanstack/react-virtual.
 */
export function useVirtualList<T>({
  items,
  estimateSize = 32,
  overscan = 5,
}: UseVirtualListOptions<T>): UseVirtualListResult<T> {
  const parentRef = React.useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan,
  });

  return {
    parentRef,
    virtualizer,
    virtualItems: virtualizer.getVirtualItems(),
    totalSize: virtualizer.getTotalSize(),
  };
}
