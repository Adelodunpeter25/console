interface ScrollToBottomProps {
  onClick: () => void;
}

/**
 * Floating "↓ Latest" button that appears when the user has scrolled up
 * during streaming and new content is arriving below the viewport.
 */
export function ScrollToBottom({ onClick }: ScrollToBottomProps) {
  return (
    <div className="relative">
      <button
        onClick={onClick}
        className="absolute -top-12 left-1/2 -translate-x-1/2 px-3 py-1.5 bg-card border border-border-strong rounded-full text-xs font-medium text-foreground-secondary shadow-lg hover:text-foreground transition-colors z-10"
      >
        ↓ Latest
      </button>
    </div>
  );
}
