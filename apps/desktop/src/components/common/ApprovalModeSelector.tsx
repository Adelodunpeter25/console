import React, { useEffect } from "react";
import type { ApprovalMode, ApprovalModeOption } from "@console/types";
import { Dropdown, DropdownItem } from "./Dropdown";
import { useProviderStore } from "../../store/useProviderStore";

interface ApprovalModeSelectorProps {
  value: ApprovalMode;
  onChange: (mode: ApprovalMode) => void;
}

/** Fallback label map used while backend modes are loading. */
const FALLBACK_LABELS: Record<ApprovalMode, string> = {
  "always-ask": "Normal",
  "accept-edits": "Accept Edits",
  "plan-mode": "Plan Mode",
  "full-access": "Bypass",
};

/**
 * Dropdown for selecting the agent's approval mode. Controls how the agent
 * handles tool permissions — from asking for every action to full autonomy.
 * Mode options are fetched from the backend so they stay in sync with the
 * server's authoritative list.
 */
export function ApprovalModeSelector({ value, onChange }: ApprovalModeSelectorProps) {
  const { approvalModes, loadingApprovalModes, loadApprovalModes } = useProviderStore();

  useEffect(() => {
    void loadApprovalModes();
  }, [loadApprovalModes]);

  const modes: ApprovalModeOption[] = approvalModes;
  const currentLabel =
    modes.find((m) => m.value === value)?.label ?? FALLBACK_LABELS[value] ?? value;

  return (
    <Dropdown label={currentLabel} heading="Permissions" width={264}>
      {loadingApprovalModes && modes.length === 0 ? (
        <div className="px-3 py-2 font-sans text-xs text-foreground-muted">
          Loading…
        </div>
      ) : (
        modes.map((mode) => (
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
        ))
      )}
    </Dropdown>
  );
}
