import React from "react";
import type { ApprovalMode } from "@console/types";
import { Dropdown, DropdownItem } from "./Dropdown";

interface ApprovalModeSelectorProps {
  value: ApprovalMode;
  onChange: (mode: ApprovalMode) => void;
}

const MODES: { value: ApprovalMode; label: string; description: string }[] = [
  { value: "always-ask", label: "Normal", description: "Ask for every action" },
  { value: "accept-edits", label: "Accept Edits", description: "Auto-approve file edits" },
  { value: "plan-mode", label: "Plan Mode", description: "Plan only, no execution" },
  { value: "full-access", label: "Bypass Permissions", description: "Run everything without asking" },
];

const LABELS: Record<ApprovalMode, string> = {
  "always-ask": "Normal",
  "accept-edits": "Accept Edits",
  "plan-mode": "Plan Mode",
  "full-access": "Bypass",
};

/**
 * Dropdown for selecting the agent's approval mode. Controls how the agent
 * handles tool permissions — from asking for every action to full autonomy.
 */
export function ApprovalModeSelector({ value, onChange }: ApprovalModeSelectorProps) {
  return (
    <Dropdown label={LABELS[value]} heading="Permissions" width={264}>
      {MODES.map((mode) => (
        <div key={mode.value}>
          <DropdownItem
            selected={value === mode.value}
            onClick={() => onChange(mode.value)}
          >
            <span className="flex flex-col gap-0.5">
              <span className="font-sans text-xs font-medium">{mode.label}</span>
              <span className="font-sans text-[10px] text-foreground-muted">
                {mode.description}
              </span>
            </span>
          </DropdownItem>
        </div>
      ))}
    </Dropdown>
  );
}
