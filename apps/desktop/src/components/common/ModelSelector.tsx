import React from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useProviderStore } from "../../store/useProviderStore";
import { Dropdown, DropdownItem, DropdownSearch } from "./Dropdown";

interface ModelSelectorProps {
  value: string | null;
  onChange: (modelId: string) => void;
}

/**
 * Dropdown for selecting an LLM model. Pulls providers from the provider
 * store and loads models on first open. Built on the shared Dropdown
 * primitives so it shares the same visual language as other selectors.
 */
export function ModelSelector({ value, onChange }: ModelSelectorProps) {
  const providers = useProviderStore((state) => state.providers);
  const modelsByProvider = useProviderStore((state) => state.modelsByProvider);
  const loadProviders = useProviderStore((state) => state.loadProviders);
  const loadModels = useProviderStore((state) => state.loadModels);
  const loadingProviders = useProviderStore((state) => state.loadingProviders);
  const loadingModels = useProviderStore((state) => state.loadingModels);
  const [search, setSearch] = React.useState("");
  const [collapsedProviders, setCollapsedProviders] = React.useState<Set<string>>(new Set());
  const query = search.trim().toLowerCase();

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  const modelsByProviderFiltered = React.useMemo(() => {
    const result: Record<string, typeof modelsByProvider[string]> = {};
    for (const provider of providers) {
      const models = modelsByProvider[provider.name] ?? [];
      result[provider.name] = !query
        ? models
        : models.filter((m) => m.id.toLowerCase().includes(query));
    }
    return result;
  }, [providers, modelsByProvider, query]);

  const allModels = React.useMemo(
    () => providers.flatMap((p) => modelsByProviderFiltered[p.name] ?? []),
    [providers, modelsByProviderFiltered],
  );

  const handleOpen = () => {
    providers.forEach((p) => {
      if (!modelsByProvider[p.name]) {
        loadModels(p.name).catch(() => {});
      }
    });
  };

  const toggleProvider = (providerId: string) => {
    setCollapsedProviders((current) => {
      const next = new Set(current);
      if (next.has(providerId)) next.delete(providerId);
      else next.add(providerId);
      return next;
    });
  };

  return (
    <Dropdown label={value ?? "Default"} heading="Models" onOpen={handleOpen} width={264}>
      <DropdownSearch value={search} onChange={setSearch} placeholder="Search models..." />
      {loadingProviders && (
        <div className="px-3 py-4 text-center text-xs text-foreground-muted">Loading...</div>
      )}
      {!loadingProviders && allModels.length === 0 && (
        <div className="px-3 py-4 text-center text-xs text-foreground-muted">
          No models available
        </div>
      )}
      {providers.map((provider) => {
        const models = modelsByProviderFiltered[provider.name] ?? [];
        const isLoading = loadingModels[provider.name];
        if (!isLoading && models.length === 0) return null;
        const collapsed = collapsedProviders.has(provider.name) && !query;
        return (
          <div key={provider.name}>
            <button
              type="button"
              onClick={() => toggleProvider(provider.name)}
              aria-expanded={!collapsed}
              className="flex w-full items-center gap-1.5 rounded-md px-1.5 pb-1 pt-2 text-left text-[10px] font-medium uppercase tracking-wider text-foreground-muted outline-none transition-colors hover:text-foreground-secondary focus-visible:text-foreground"
            >
              {collapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
              <span className="truncate">{provider.displayName}</span>
              <span className="ml-auto text-[9px] normal-case tracking-normal text-foreground-muted/70">
                {models.length}
              </span>
            </button>
            {!collapsed &&
              (isLoading ? (
                <div className="px-3 py-2 text-xs text-foreground-muted">Loading models...</div>
              ) : (
                models.map((model) => (
                  <DropdownItem
                    key={model.id}
                    selected={value === model.id}
                    onClick={() => onChange(model.id)}
                  >
                    {model.id}
                  </DropdownItem>
                ))
              ))}
          </div>
        );
      })}
    </Dropdown>
  );
}
