import { ArrowLeft, Wifi } from "lucide-react";

export type SettingsSection = "connection";

interface SettingsSidebarProps {
  active: SettingsSection;
  onSelect: (section: SettingsSection) => void;
  onBack: () => void;
  /** Current sidebar width (px) — set from the parent's resizable panel. */
  width: number;
}

/**
 * Settings navigation sidebar.
 *
 * Shows a "Back to App" button at the top, followed by a list of settings
 * sections. Currently only "Connection" is available; the structure is
 * ready for more sections (appearance, models, etc.) to be added.
 */
export function SettingsSidebar({ active, onSelect, onBack, width }: SettingsSidebarProps) {
  const sections: { id: SettingsSection; label: string; icon: React.ReactNode }[] = [
    { id: "connection", label: "Connection", icon: <Wifi size={16} /> },
  ];

  return (
    <div
      style={{ width }}
      className="bg-sidebar border-r border-border flex flex-col h-full shrink-0"
    >
      {/* Back button */}
      <div className="px-3 h-12 flex items-center border-b border-border shrink-0">
        <button
          onClick={onBack}
          className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-sm font-medium text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
        >
          <ArrowLeft size={16} />
          Back to App
        </button>
      </div>

      {/* Section list */}
      <div className="flex-1 overflow-y-auto px-2 py-3">
        <span className="text-xs font-semibold text-foreground-muted uppercase tracking-wider px-2 mb-2 block">
          Settings
        </span>
        <div className="space-y-0.5">
          {sections.map((section) => (
            <button
              key={section.id}
              onClick={() => onSelect(section.id)}
              className={`w-full flex items-center gap-2.5 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors text-left ${
                active === section.id
                  ? "bg-white/10 text-foreground border border-border"
                  : "text-foreground-secondary hover:text-foreground hover:bg-white/5 border border-transparent"
              }`}
            >
              {section.icon}
              {section.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
