import type { LucideIcon } from "lucide-react";
import { Brain } from "lucide-react";

interface EmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description: string;
}

/**
 * Centered empty-state placeholder with an icon, heading, and subtext.
 * Used for the "No Session Selected" state and other empty views.
 */
export function EmptyState({ icon: Icon = Brain, title, description }: EmptyStateProps) {
  return (
    <div className="h-full w-full flex items-center justify-center bg-screen">
      <div className="text-center max-w-sm">
        <div className="w-14 h-14 rounded-2xl bg-card border border-border flex items-center justify-center mx-auto mb-4">
          <Icon size={26} className="text-foreground-muted" />
        </div>
        <p className="text-foreground text-base font-semibold mb-1.5">{title}</p>
        <p className="text-foreground-secondary text-sm">{description}</p>
      </div>
    </div>
  );
}
