import React from "react";
import { ChevronDown, Check } from "lucide-react";
import { useProviderStore } from "../store";

interface ModelSelectorProps {
  value: string | null;
  onChange: (modelId: string) => void;
}

/**
 * Dropdown for selecting an LLM model. Pulls providers from the provider
 * store and loads models on first open.
 */
export function ModelSelector({ value, onChange }: ModelSelectorProps) {
  const { providers, modelsByProvider, loadProviders, loadModels, loadingProviders, loadingModels } =
    useProviderStore();
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const allModels = React.useMemo(() => {
    return providers.flatMap((p) => {
      const models = modelsByProvider[p.name] ?? [];
      return models.map((m) => ({ ...m, providerName: p.displayName }));
    });
  }, [providers, modelsByProvider]);

  const selectedLabel = value ?? "Default";

  const handleOpen = () => {
    if (!open) {
      providers.forEach((p) => {
        if (!modelsByProvider[p.name]) {
          loadModels(p.name).catch(() => {});
        }
      });
    }
    setOpen(!open);
  };

  return (
    <div ref={ref} className="relative">
      <button
        onClick={handleOpen}
        className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors"
      >
        {selectedLabel}
        <ChevronDown size={14} className={`transition-transform ${open ? "rotate-180" : ""}`} />
      </button>

      {open && (
        <div className="absolute bottom-full left-0 mb-2 w-64 bg-card border border-border-strong rounded-xl shadow-2xl overflow-hidden z-50">
          <div className="px-3 py-2 border-b border-border">
            <span className="text-xs font-semibold text-foreground-muted uppercase tracking-wider">
              Models
            </span>
          </div>
          <div className="max-h-64 overflow-y-auto">
            {loadingProviders && (
              <div className="px-3 py-4 text-center text-xs text-foreground-muted">Loading...</div>
            )}
            {!loadingProviders && allModels.length === 0 && (
              <div className="px-3 py-4 text-center text-xs text-foreground-muted">
                No models available
              </div>
            )}
            {providers.map((provider) => {
              const models = modelsByProvider[provider.name] ?? [];
              const isLoading = loadingModels[provider.name];
              return (
                <div key={provider.name}>
                  <div className="px-3 py-1.5 text-xs font-semibold text-foreground-muted uppercase tracking-wider bg-white/[0.02]">
                    {provider.displayName}
                  </div>
                  {isLoading ? (
                    <div className="px-3 py-2 text-xs text-foreground-muted">Loading models...</div>
                  ) : (
                    models.map((model) => (
                      <button
                        key={model.id}
                        onClick={() => {
                          onChange(model.id);
                          setOpen(false);
                        }}
                        className="w-full flex items-center justify-between px-3 py-2 text-sm text-foreground-secondary hover:text-foreground hover:bg-white/5 transition-colors text-left"
                      >
                        <span className="truncate font-mono text-xs">{model.id}</span>
                        {value === model.id && <Check size={14} className="text-success shrink-0" />}
                      </button>
                    ))
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
