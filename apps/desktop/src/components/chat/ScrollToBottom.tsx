interface ScrollToBottomProps {
  onClick: () => void;
}

/**
 * Floating "↓ Latest" button that appears whenever the user has scrolled
 * away from the bottom of the chat — whether the agent is streaming or not.
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
