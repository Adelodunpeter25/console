import React from "react";

interface UseCustomChatVirtualizerOptions<T> {
  items: T[];
  estimateSize?: (item: T, index: number) => number;
  overscan?: number;
}

/**
 * Custom windowing virtualization hook tailored for chat applications.
 *
 * Renders visible items in normal document flow bounded by top and bottom spacer divs.
 * Includes scroll-offset compensation when topSpacerHeight changes during backward
 * scrolling to eliminate blank screens and visual layout jumps.
 */
export function useCustomChatVirtualizer<T>({
  items,
  estimateSize = () => 150,
  overscan = 10,
}: UseCustomChatVirtualizerOptions<T>) {
  const parentRef = React.useRef<HTMLDivElement>(null);
  const sizeMapRef = React.useRef<Map<number, number>>(new Map());
  const [scrollState, setScrollState] = React.useState({
    scrollTop: 0,
    clientHeight: 0,
  });

  const handleScroll = React.useCallback(() => {
    if (!parentRef.current) return;
    const { scrollTop, clientHeight } = parentRef.current;
    setScrollState({ scrollTop, clientHeight });
  }, []);

  // Measure dynamic height of visible items via ResizeObserver
  const resizeObserverRef = React.useRef<ResizeObserver | null>(null);
  if (!resizeObserverRef.current && typeof window !== "undefined") {
    resizeObserverRef.current = new ResizeObserver((entries) => {
      let changed = false;
      for (const entry of entries) {
        const indexAttr = entry.target.getAttribute("data-virtual-index");
        if (indexAttr !== null) {
          const index = parseInt(indexAttr, 10);
          const height = entry.contentRect.height;
          if (height > 0 && sizeMapRef.current.get(index) !== height) {
            sizeMapRef.current.set(index, height);
            changed = true;
          }
        }
      }
      if (changed && parentRef.current) {
        const { scrollTop, clientHeight } = parentRef.current;
        setScrollState({ scrollTop, clientHeight });
      }
    });
  }

  React.useEffect(() => {
    return () => {
      resizeObserverRef.current?.disconnect();
    };
  }, []);

  const measureRef = React.useCallback((index: number, node: HTMLElement | null) => {
    if (!node) return;
    node.setAttribute("data-virtual-index", String(index));
    const height = node.getBoundingClientRect().height;
    if (height > 0 && sizeMapRef.current.get(index) !== height) {
      sizeMapRef.current.set(index, height);
    }
    resizeObserverRef.current?.observe(node);
  }, []);

  const getItemSize = React.useCallback(
    (index: number) => {
      return sizeMapRef.current.get(index) ?? estimateSize(items[index]!, index);
    },
    [items, estimateSize],
  );

  // Calculate cumulative start offsets for all items
  const { offsets, totalSize } = React.useMemo(() => {
    const offsets: number[] = new Array(items.length);
    let current = 0;
    for (let i = 0; i < items.length; i++) {
      offsets[i] = current;
      current += sizeMapRef.current.get(i) ?? estimateSize(items[i]!, i);
    }
    return { offsets, totalSize: current };
  }, [items, estimateSize, scrollState]);

  // Determine window of visible items
  const { startIndex, endIndex, topSpacerHeight, bottomSpacerHeight } = React.useMemo(() => {
    if (items.length === 0) {
      return { startIndex: 0, endIndex: 0, topSpacerHeight: 0, bottomSpacerHeight: 0 };
    }

    const { scrollTop, clientHeight } = scrollState;
    const viewportEnd = scrollTop + (clientHeight || 800);

    let start = 0;
    while (start < items.length - 1 && offsets[start]! + getItemSize(start) < scrollTop) {
      start++;
    }
    start = Math.max(0, start - overscan);

    let end = start;
    while (end < items.length - 1 && offsets[end]! < viewportEnd) {
      end++;
    }
    end = Math.min(items.length - 1, end + overscan);

    const topSpacer = offsets[start] ?? 0;
    const endItemBottom = (offsets[end] ?? 0) + getItemSize(end);
    const bottomSpacer = Math.max(0, totalSize - endItemBottom);

    return {
      startIndex: start,
      endIndex: end,
      topSpacerHeight: topSpacer,
      bottomSpacerHeight: bottomSpacer,
    };
  }, [items.length, scrollState, offsets, totalSize, getItemSize, overscan]);

  // Compensate scrollTop when topSpacerHeight shifts as preceding items above the fold are measured
  const prevTopSpacerRef = React.useRef(topSpacerHeight);
  React.useLayoutEffect(() => {
    const prev = prevTopSpacerRef.current;
    prevTopSpacerRef.current = topSpacerHeight;
    const delta = topSpacerHeight - prev;
    if (delta !== 0 && parentRef.current && scrollState.scrollTop > 0) {
      parentRef.current.scrollTop += delta;
    }
  }, [topSpacerHeight, scrollState.scrollTop]);

  const scrollToEnd = React.useCallback(() => {
    if (parentRef.current) {
      parentRef.current.scrollTop = parentRef.current.scrollHeight;
    }
  }, []);

  return {
    parentRef,
    handleScroll,
    startIndex,
    endIndex,
    topSpacerHeight,
    bottomSpacerHeight,
    measureRef,
    scrollToEnd,
  };
}
