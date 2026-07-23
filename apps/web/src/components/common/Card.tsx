import React from "react";

export interface CardProps extends React.HTMLAttributes<HTMLDivElement> {}

export const Card = React.forwardRef<HTMLDivElement, CardProps>(
  ({ className = "", ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={`rounded-md border border-border bg-card text-card-foreground shadow-sm ${className}`}
        {...props}
      />
    );
  },
);
Card.displayName = "Card";
