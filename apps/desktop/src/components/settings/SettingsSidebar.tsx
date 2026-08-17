import { ArrowLeft, MessagesSquare, User, Wifi } from "lucide-react";

export type SettingsSection = "account" | "connection" | "deleted-chats";

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
 * sections. "Account" is first (most fundamental), followed by "Connection".
 */
export function SettingsSidebar({ active, onSelect, onBack, width }: SettingsSidebarProps) {
  const sections: { id: SettingsSection; label: string; icon: React.ReactNode }[] = [
    {
      id: "account",
      label: "Account",
      icon: <User size={14} className="shrink-0 text-current" />,
    },
    {
      id: "connection",
      label: "Connection",
      icon: <Wifi size={14} className="shrink-0 text-current" />,
    },
    {
      id: "deleted-chats",
      label: "Deleted chats",
      icon: <MessagesSquare size={14} className="shrink-0 text-current" />,
    },
  ];

  return (
    <div
      style={{ width }}
      className="bg-sidebar border-r border-border flex flex-col h-full shrink-0"
    >
      {/* Back button */}
      <div className="px-2 h-11 flex items-center border-b border-border/50 shrink-0">
        <button
          onClick={onBack}
          className="flex items-center gap-2 px-2.5 py-1 rounded-md text-[13px] font-normal text-white/50 hover:text-white hover:bg-white/[0.04] transition-colors"
        >
          <ArrowLeft size={14} />
          Back to App
        </button>
      </div>

      {/* Section list */}
      <div className="flex-1 overflow-y-auto px-2 py-2">
        <div className="space-y-1">
          {sections.map((section) => (
            <button
              key={section.id}
              onClick={() => onSelect(section.id)}
              className={`w-full flex items-center gap-3 px-3 py-1.5 rounded-md text-[13px] font-normal transition-all text-left ${
                active === section.id
                  ? "bg-white/[0.08] text-white"
                  : "text-white/60 hover:text-white hover:bg-white/[0.04]"
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
